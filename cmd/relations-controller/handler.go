package main

import (
	log "github.com/Sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"gitlab.ilabt.imec.be/sborny/orcon/internal/deploymentpatch"
	"gitlab.ilabt.imec.be/sborny/orcon/internal/orconlib"
)

// Handler interface contains the methods that are required
type Handler interface {
	Init() error
	ServiceCreated(obj interface{})
	DeploymentCreated(obj interface{})
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

func (t *TestHandler) addBaseURL(service *core_v1.Service, deployments *[]appsv1.Deployment) {
	relationConfig := map[string]string{
		"BASE_URL": service.Spec.ExternalName,
	}
	for _, origDeployment := range *deployments {
		deployment := deploymentpatch.New(origDeployment)
		deployment.AppendToPodEnvironment(relationConfig)
		// TODO: The next line is only for benchmarking, remove
		// after benchmarks are finished.
		deployment.AppendToPodLabels(relationConfig)
		patch, err := deployment.GetPatchBytes()
		if err != nil {
			log.Errorf("Patching failed, cannot encode patch %v", err)
			continue
		}
		if len(patch) == 0 {
			log.Infof("Nothing to patch..")
			continue
		}
		log.Infof("Patching deployment %v, patch=%v", origDeployment.Name, string(patch))
		_, err = t.clientset.AppsV1().Deployments("k8s-tengu-test").Patch(
			origDeployment.Name,
			types.JSONPatchType,
			patch,
		)
		if err != nil {
			log.Errorf("Patching deployment failed: %v", err)
		} else {
			log.Infof("Patching deployment succeeded")
		}
	}
}

// ServiceCreated is called when a service is created
func (t *TestHandler) ServiceCreated(obj interface{}) {
	log.Info("TestHandler.ServiceCreated")
	// assert the type to a Service object to pull out relevant data
	service := obj.(*core_v1.Service)
	log.Infof("    ResourceVersion: %s", service.ObjectMeta.ResourceVersion)
	log.Infof("    ExternalName: %s", service.Spec.ExternalName)

	deployments := orconlib.GetRelatedDeployments(service.Name, t.clientset)
	log.Infof("Found related %v deployments.", len(*deployments))
	t.addBaseURL(service, deployments)
}

// DeploymentCreated is called when an deployment is created
func (t *TestHandler) DeploymentCreated(obj interface{}) {
	log.Info("TestHandler.ObjectCreated")
	// assert the type to a Service object to pull out relevant data
	deployment := obj.(*appsv1.Deployment)
	log.Infof("    ResourceVersion: %s", deployment.ObjectMeta.ResourceVersion)
	log.Infof("    Name: %s", deployment.ObjectMeta.Name)

	servicename := deployment.Labels["tengu.io/relations"]
	if servicename == "" {
		log.Infof("Deployment %v has no relationships", deployment.ObjectMeta.Name)
		return
	}
	service, err := t.clientset.CoreV1().Services("k8s-tengu-test").Get(servicename, metav1.GetOptions{})
	if err != nil {
		log.Warnf("Couldn't get service %v: %v", servicename, err)
		return
	}
	deployments := []appsv1.Deployment{*deployment}
	t.addBaseURL(service, &deployments)
}

// ObjectDeleted is called when an object is deleted
func (t *TestHandler) ObjectDeleted(obj interface{}) {
	log.Info("TestHandler.ObjectDeleted")
}

// ObjectUpdated is called when an object is updated
func (t *TestHandler) ObjectUpdated(objOld, objNew interface{}) {
	log.Info("TestHandler.ObjectUpdated")
}
