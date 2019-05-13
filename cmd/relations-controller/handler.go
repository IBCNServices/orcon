package main

import (
	"strconv"
	"strings"

	log "github.com/Sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Handler interface contains the methods that are required
type Handler interface {
	Init() error
	ObjectCreated(obj interface{})
	ObjectDeleted(obj interface{})
	ObjectUpdated(objOld, objNew interface{})
}

// TestHandler is a sample implementation of Handler
type TestHandler struct {
	clientset kubernetes.Interface
}

// Init handles any handler initialization
func (t *TestHandler) Init() error {
	log.Info("TestHandler.Init")
	return nil
}

//
// TODO: MOVE TO SHARED LIB
//

// PatchedDeployment is used to modify a Deployment resource using jsonpatch
type PatchedDeployment struct {
	deployment appsv1.Deployment
	patchList  []patchOperation

	// Internal vars
	podLabelsEnsured      bool
	podAnnotationsEnsured bool
	podEnvironmentEnsured bool
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

func newPatchedDeployment(deployment appsv1.Deployment) *PatchedDeployment {
	pd := &PatchedDeployment{
		deployment: deployment,
	}
	return pd
}

func (d *PatchedDeployment) ensurePodLabelsExist() {
	if !d.podLabelsEnsured {
		if len(d.deployment.Spec.Template.Labels) == 0 {
			d.patchList = append(d.patchList, patchOperation{
				Op:    "add",
				Path:  "/spec/template/metadata/labels",
				Value: struct{}{},
			})
		}
		d.podLabelsEnsured = true
	}
}

func (d *PatchedDeployment) ensurePodAnnotationsExist() {
	if !d.podAnnotationsEnsured {
		if len(d.deployment.Spec.Template.Labels) == 0 {
			d.patchList = append(d.patchList, patchOperation{
				Op:    "add",
				Path:  "/spec/template/metadata/annotations",
				Value: struct{}{},
			})
		}
		d.podAnnotationsEnsured = true
	}
}

func (d *PatchedDeployment) ensurePodEnvironmentExists() {
	if !d.podEnvironmentEnsured {
		for count := range d.deployment.Spec.Template.Spec.Containers {
			if len(d.deployment.Spec.Template.Spec.Containers[count].Env) == 0 {
				d.patchList = append(d.patchList, patchOperation{
					Op:    "add",
					Path:  "/spec/template/spec/containers/" + strconv.Itoa(count) + "/env",
					Value: struct{}{},
				})
			}
		}
		d.podEnvironmentEnsured = true
	}
}

// AppendToPodAnnotations adds given map of annotations to the pod template
func (d *PatchedDeployment) AppendToPodAnnotations(config map[string]string) {
	d.ensurePodAnnotationsExist()
	for key, value := range config {
		// https://stackoverflow.com/questions/36147137/kubernetes-api-add-label-to-pod#comment98654379_36163917
		escapedKey := strings.Replace(key, "~", "~0", -1)
		escapedKey = strings.Replace(escapedKey, "/", "~1", -1)
		d.patchList = append(d.patchList, patchOperation{
			Op:    "add",
			Path:  "/spec/template/metadata/annotations/" + escapedKey,
			Value: value,
		})
	}
}

// AppendToPodLabels appends the given map of labels to the pod template
func (d *PatchedDeployment) AppendToPodLabels(config map[string]string) {
	d.ensurePodLabelsExist()
	for key, value := range config {
		// https://stackoverflow.com/questions/36147137/kubernetes-api-add-label-to-pod#comment98654379_36163917
		escapedKey := strings.Replace(key, "~", "~0", -1)
		escapedKey = strings.Replace(escapedKey, "/", "~1", -1)
		d.patchList = append(d.patchList, patchOperation{
			Op:    "add",
			Path:  "/spec/template/metadata/labels/" + escapedKey,
			Value: value,
		})
	}
}

// AppendToPodEnvironment adds the map of environment variables to all containers that are in the original podspec
func (d *PatchedDeployment) AppendToPodEnvironment(config map[string]string) {
	d.ensurePodEnvironmentExists()
	count := 0
	for key, value := range config {
		d.patchList = append(d.patchList, patchOperation{
			Op:    "add",
			Path:  "/spec/template/spec/containers/" + strconv.Itoa(count) + "/env/" + key,
			Value: value,
		})
		count++
	}
}

// GetRelatedDeployments returns the deployments related to the resource with given name.
func GetRelatedDeployments(name string, clientset kubernetes.Interface) *[]appsv1.Deployment {
	deploymentList, err := clientset.AppsV1().Deployments("k8s-tengu-test").List(metav1.ListOptions{
		// https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#ListOptions
		LabelSelector: "tengu.io/relations=production",
	})
	if err != nil {
		log.Warn("getting related deployments failed: %v", err)
		return nil
	}
	return &deploymentList.Items
}

// ObjectCreated is called when an object is created
func (t *TestHandler) ObjectCreated(obj interface{}) {
	log.Info("TestHandler.ObjectCreated")
	// assert the type to a Service object to pull out relevant data
	service := obj.(*core_v1.Service)
	log.Infof("    ResourceVersion: %s", service.ObjectMeta.ResourceVersion)
	log.Infof("    ExternalName: %s", service.Spec.ExternalName)
	log.Infof("    Phase: %s", service.ObjectMeta.Annotations["BASE_URL"])

	relationConfig := map[string]string{
		"BASE_URL": service.Spec.ExternalName,
	}

	deployments := GetRelatedDeployments(service.Name, t.clientset)
	for _, origDeployment := range *deployments {
		deployment := newPatchedDeployment(origDeployment)
		deployment.AppendToPodEnvironment(relationConfig)
		// TODO: The next line is only for benchmarking, remove
		// after benchmarks are finished.
		deployment.AppendToPodLabels(relationConfig)
	}
}

// ObjectDeleted is called when an object is deleted
func (t *TestHandler) ObjectDeleted(obj interface{}) {
	log.Info("TestHandler.ObjectDeleted")
}

// ObjectUpdated is called when an object is updated
func (t *TestHandler) ObjectUpdated(objOld, objNew interface{}) {
	log.Info("TestHandler.ObjectUpdated")
}
