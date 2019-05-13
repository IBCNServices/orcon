package main

import (
	log "github.com/Sirupsen/logrus"

	core_v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"gitlab.ilabt.imec.be/sborny/orcon/internal/deploymentpatch"
	"gitlab.ilabt.imec.be/sborny/orcon/internal/orconlib"
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

	deployments := orconlib.GetRelatedDeployments(service.Name, t.clientset)
	for _, origDeployment := range *deployments {
		deployment := deploymentpatch.New(origDeployment)
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
