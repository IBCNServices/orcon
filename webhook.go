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
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
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

const (
	//admissionWebhookAnnotationInjectKey = "tengu-injector-webhook/inject"
	admissionWebhookAnnotationInjectKey    = "consumes"
	admissionWebhookAnnotationRelationsKey = "relations"
	admissionWebhookAnnotationStatusKey    = "tengu-injector-webhook/status"
)

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
	Value interface{} `json:"value,omitempty"`
}

func init() {
	_ = corev1.AddToScheme(runtimeScheme)
	_ = admissionregistrationv1beta1.AddToScheme(runtimeScheme)

	_ = v1.AddToScheme(runtimeScheme)
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
func mutationRequired(ignoredList []string, metadata *metav1.ObjectMeta) bool {
	// Skip special kubernetes system namespaces
	for _, namespace := range ignoredList {
		if metadata.Namespace == namespace {
			glog.Infof("Skip mutation for %v for it's in special namespace: %v", metadata.Name, metadata.Namespace)
			return false
		}
	}
	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	status := annotations[admissionWebhookAnnotationStatusKey]

	// Determine whether to perform mutation based on annotation for the target resource
	var required bool
	if strings.ToLower(status) == "injected" {
		required = false
	} else {
		if _, ok := annotations[admissionWebhookAnnotationInjectKey]; ok {
			required = true
		} else {
			required = false
		}
	}

	glog.Infof("Mutation policy for %v/%v: status: %q required:%v", metadata.Namespace, metadata.Name, status, required)
	return required
}

func getService(service string) *v1.Service {
	svc, err := clientset.CoreV1().Services(metav1.NamespaceDefault).Get(service, metav1.GetOptions{})
	if err != nil {
		glog.Infof("Service (%s) does not exist.", service)
		return nil
	}
	glog.Infof("Service (%s) exists!", service)
	return svc
}

func fillEnvVars(envVars []corev1.EnvVar, service *v1.Service) {
	annotations := service.GetAnnotations()

	for _, envVar := range envVars {
		if _, ok := annotations[envVar.Name]; ok {
			envVar.Value = annotations[envVar.Name]
		}
	}
}

func populateEnvVars(annotations map[string]string) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}

	tenguInterface := annotations[admissionWebhookAnnotationInjectKey]
	glog.Infof("tenguInterface: %s", tenguInterface)

	envVar := corev1.EnvVar{
		Name:  "TENGU_REQUIRED_VARS",
		Value: strings.Join(interfaceLookupDict()[tenguInterface], ","),
	}
	envVars = append(envVars, envVar)

	relationName := annotations[admissionWebhookAnnotationRelationsKey]
	glog.Infof("relationName: %s", relationName)
	if relationName != "" {
		svc := getService(relationName)
		if svc != nil {
			fillEnvVars(envVars, svc)
		}
	}

	return envVars
}

func addInitContainer(target, added []corev1.Container, basePath string, envVars []corev1.EnvVar) (patch []patchOperation) {
	glog.Infof("Length of added containers: %d", len(added))
	glog.Infof("Length of envVars: %d", len(envVars))
	first := len(target) == 0
	var value interface{}
	for _, add := range added {
		add.Env = append(add.Env, envVars...)
		value = add
		path := basePath
		if first {
			glog.Infof("This is first")
			first = false
			value = []corev1.Container{add}
		} else {
			path = path + "/-"
			glog.Infof("This is else, path: %s", path)
		}
		patch = append(patch, patchOperation{
			Op:    "add",
			Path:  path,
			Value: value,
		})
	}
	return patch
}

func updateAnnotation(target map[string]string, added map[string]string) (patch []patchOperation) {
	for key, value := range added {
		if target == nil || target[key] == "" {
			target = map[string]string{}
			patch = append(patch, patchOperation{
				Op:   "add",
				Path: "/metadata/annotations",
				Value: map[string]string{
					key: value,
				},
			})
		} else {
			patch = append(patch, patchOperation{
				Op:    "replace",
				Path:  "/metadata/annotations/" + key,
				Value: value,
			})
		}
	}
	return patch
}

func createPatch(pod *corev1.Pod, initcontainerConfig *Config, annotations map[string]string) ([]byte, error) {
	var patch []patchOperation

	envVars := populateEnvVars(pod.GetAnnotations())
	patch = append(patch, addInitContainer(pod.Spec.InitContainers, initcontainerConfig.InitContainers, "/spec/initContainers", envVars)...)
	patch = append(patch, updateAnnotation(pod.Annotations, annotations)...)

	return json.Marshal(patch)
}

func (whsvr *WebhookServer) mutate(ar *v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	req := ar.Request
	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		glog.Errorf("Could not unmarshal raw object: %v", err)
		return &v1beta1.AdmissionResponse{
			Result: &metav1.Status{
				Message: err.Error(),
			},
		}
	}

	glog.Infof("AdmissionReview for Kind=%v, Namespace=%v Name=%v (%v) UID=%v patchOperation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, pod.Name, req.UID, req.Operation, req.UserInfo)

	//determine whether to perform mutation
	if !mutationRequired(ignoredNamespaces, &pod.ObjectMeta) {
		glog.Infof("Skipping mutation for %s/%s due to policy check", pod.Namespace, pod.Name)
		return &v1beta1.AdmissionResponse{
			Allowed: true,
		}
	}
	// Workaround: https://github.com/kubernetes/kubernetes/issues/57982
	applyDefaultsWorkaround(whsvr.initcontainerConfig.InitContainers)
	annotations := map[string]string{admissionWebhookAnnotationStatusKey: "injected"}
	patchBytes, err := createPatch(&pod, whsvr.initcontainerConfig, annotations)

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
	glog.Infof("Ready to write reponse ...")
	if _, err := w.Write(resp); err != nil {
		glog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}
