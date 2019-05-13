package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/golang/glog"
	"gopkg.in/yaml.v2"
	"k8s.io/api/admission/v1beta1"
	admissionregistrationv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()

	defaulter = runtime.ObjectDefaulter(runtimeScheme)

	clientset = createK8sClient()
)

var ignoredNamespaces = []string{
	metav1.NamespaceSystem,
	metav1.NamespacePublic,
}

//Use this map to determine REQUIRED_VARS
var interfaceLookupDict = func() map[string][]string {
	return map[string][]string{
		"sse": []string{"BASE_URL"},
	}
}

//Config ...
type Config struct {
	InitContainers []corev1.Container `yaml:"initContainers"`
}

//WebhookServer ...
type WebhookServer struct {
	initcontainerConfig *Config
	server              *http.Server
}

//WhSvrParameters ...
type WhSvrParameters struct {
	port                 int    // webhook server port
	certFile             string // path to the x509 certificate for https
	keyFile              string // path to the x509 private key matching `CertFile`
	initcontainerCfgFile string // path to the initcontainer injector configuration file
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

func init() {
	_ = corev1.AddToScheme(runtimeScheme)
	_ = admissionregistrationv1beta1.AddToScheme(runtimeScheme)

	_ = corev1.AddToScheme(runtimeScheme)
}

func createK8sClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	return clientset
}

// (https://github.com/kubernetes/kubernetes/issues/57982)
func applyDefaultsWorkaround(containers []corev1.Container) {
	defaulter.Default(&corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: containers,
		},
	})
}

func loadConfig(configFile string) (*Config, error) {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	glog.Infof("New configuration: sha256sum %x", sha256.Sum256(data))

	var cfg Config

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Check whether the target resource needs to be mutated
func mutationRequired(ignoredList []string, metadata *metav1.ObjectMeta) []string {
	glog.Infof("Called")
	processingRequired := []string{}
	// Skip special kubernetes system namespaces
	for _, namespace := range ignoredList {
		if metadata.Namespace == namespace {
			glog.Infof("Skip mutation for %v for it's in special namespace: %v", metadata.Name, metadata.Namespace)
			return processingRequired
		}
	}
	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	labels := metadata.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	status := strings.ToLower(annotations["injector.tengu.io/status"])
	consumes := strings.ToLower(labels["tengu.io/consumes"])
	provides := strings.ToLower(labels["tengu.io/provides"])

	glog.Infof("%s; %s; %s", status, consumes, provides)

	if consumes != "" && !(status == "injected") {
		processingRequired = append(processingRequired, "consumes")
	}
	if provides != "" {
		processingRequired = append(processingRequired, "provides")
	}

	glog.Infof("Mutation policy for %v/%v: status: %q required: %v", metadata.Namespace, metadata.Name, status, processingRequired)
	return processingRequired
}

// getValidService will check if the service exists and has a correct interface
func getValidService(service string, interfaceName string, namespace string) *corev1.Service {
	svc, err := clientset.CoreV1().Services(namespace).Get(service, metav1.GetOptions{})
	if err != nil {
		glog.Infof("Service (%s) does not exist.", service)
		glog.Infof("%s", err.Error())
		return nil
	}
	glog.Infof("Service (%s) exists!", service)
	labels := svc.GetLabels()
	if _, ok := labels["tengu.io/provides"]; ok {
		if labels["tengu.io/provides"] != interfaceName {
			glog.Infof("Service (%s) does not have matching interface: %s", service, interfaceName)
			return nil
		}
	} else {
		glog.Infof("Service (%s) does not have labels: %s", service, "tengu.io/provides")
		return nil
	}
	glog.Infof("Service (%s) matched", service)

	return svc
}

func fillEnvVars(envVars *[]corev1.EnvVar, service *corev1.Service, interfaceName string) {
	annotations := service.GetAnnotations()

	for _, env := range interfaceLookupDict()[interfaceName] {
		glog.Infof("Checking if annotation key (%s) exists", env)
		if _, ok := annotations[env]; ok {
			glog.Infof("Key exists, adding to envVars")
			*envVars = append(*envVars, corev1.EnvVar{
				Name:  env,
				Value: annotations[env],
			})
		} else {
			glog.Infof("Key does not exist")
		}
	}
}

func populateEnvVars(labels map[string]string, namespace string) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}

	tenguInterface := labels["tengu.io/consumes"]
	glog.Infof("tenguInterface: %s", tenguInterface)

	envVar := corev1.EnvVar{
		Name:  "TENGU_REQUIRED_VARS",
		Value: strings.Join(interfaceLookupDict()[tenguInterface], ","),
	}
	envVars = append(envVars, envVar)

	// relationName := labels["tengu.io/relations"]
	// glog.Infof("Looking for service '%s' in namespace '%s'", relationName, namespace)
	// if relationName != "" {
	// 	svc := getValidService(relationName, tenguInterface, namespace)
	// 	if svc != nil {
	// 		fillEnvVars(&envVars, svc, tenguInterface)
	// 	}
	// }
	return envVars
}

func addInitContainer(target, added []corev1.Container, basePath string, envVars []corev1.EnvVar) (patch []patchOperation) {
	first := len(target) == 0
	var value interface{}
	for _, add := range added {
		add.Env = append(add.Env, envVars...)
		value = add
		path := basePath
		if first {
			first = false
			value = []corev1.Container{add}
		} else {
			path = path + "/-"
		}
		patch = append(patch, patchOperation{
			Op:    "add",
			Path:  path,
			Value: value,
		})
	}
	return patch
}

func createPatch(namespace string, deployment *appsv1.Deployment, initcontainerConfig *Config, annotations map[string]string, labels map[string]string) ([]byte, error) {
	var patch []patchOperation

	envVars := populateEnvVars(deployment.GetLabels(), namespace)
	patch = append(
		patch,
		addInitContainer(deployment.Spec.Template.Spec.InitContainers, initcontainerConfig.InitContainers,
			"/spec/template/spec/initContainers",
			envVars)...)
	patch = append(
		patch,
		updateAnnotation(deployment.Spec.Template.Annotations, annotations)...)
	// patch = append(patch, addEnvironmentAsLabels(deployment.Labels, envVars)...)
	patch = append(patch, addEnvContainer(deployment.Spec.Template.Spec.Containers, "/spec/template/spec/containers", envVars)...)
	return json.Marshal(patch)
}

func (whsvr *WebhookServer) mutate(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	req := ar.Request
	// TODO: we currently only support Deployments. We should make this more
	// generic so we can also support individual pods etc.
	var deployment appsv1.Deployment
	glog.Infof(string(req.Object.Raw))
	if err := json.Unmarshal(req.Object.Raw, &deployment); err != nil {
		glog.Errorf("Could not unmarshal raw object: %v", err)
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	glog.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v (%v) UID=%v patchOperation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, deployment.Name, req.UID, req.Operation, req.UserInfo)

	//determine whether to perform mutation
	processingRequired := mutationRequired(ignoredNamespaces, &deployment.ObjectMeta)
	for _, action := range processingRequired {
		if action == "consumes" {
			// Workaround: https://github.com/kubernetes/kubernetes/issues/57982
			applyDefaultsWorkaround(whsvr.initcontainerConfig.InitContainers)
			annotations := map[string]string{"injector.tengu.io/status": "injected"}
			labels := map[string]string{"base-url": "injected"}
			patchBytes, err := createPatch(req.Namespace, &deployment, whsvr.initcontainerConfig, annotations, labels)

			if err != nil {
				return &v1beta1.AdmissionResponse{
					Result: &metav1.Status{
						Message: err.Error(),
					},
				}
			}

			glog.Infof("AdmissionResponse: patch=%v\n", string(patchBytes))
			return &v1beta1.AdmissionResponse{
				Allowed: true,
				Patch:   patchBytes,
				PatchType: func() *v1beta1.PatchType {
					pt := v1beta1.PatchTypeJSONPatch
					return &pt
				}(),
			}
		} else if action == "provides" {
			// provides side is handled the `relations-controller`
		} else {
			glog.Warningf("Action %s not recognized", action)
		}
	}

	glog.Infof("Not mutating %s/%s", deployment.Namespace, deployment.Name)
	return &v1beta1.AdmissionResponse{
		Allowed: true,
	}
}

func (whsvr *WebhookServer) serve(w http.ResponseWriter, r *http.Request) {
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		glog.Error("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	//verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		glog.Errorf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect application/json", http.StatusUnsupportedMediaType)
		return
	}

	var admissionResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		glog.Errorf("Can't decode body: %v", err)
		admissionResponse = &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	} else {
		admissionResponse = whsvr.mutate(&ar)
	}

	admissionReview := v1beta1.AdmissionReview{}
	if admissionResponse != nil {
		admissionReview.Response = admissionResponse
		if ar.Request != nil {
			admissionReview.Response.UID = ar.Request.UID
		}
	}

	resp, err := json.Marshal(admissionReview)
	if err != nil {
		glog.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}
	glog.Infof("Ready to write response ...")
	if _, err := w.Write(resp); err != nil {
		glog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}
