package main

import (
	"strings"

	log "github.com/Sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	core_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"gitlab.ilabt.imec.be/tengu/orcon-lennart/internal/deploymentpatch"
	"gitlab.ilabt.imec.be/tengu/orcon-lennart/internal/orconlib"
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

func (t *TestHandler) addBaseURL(service *core_v1.Service, deployments *[]appsv1.Deployment, ctxLog *log.Entry) {
	relationConfig := map[string]string{
		strings.ToUpper(service.Labels["tengu.io/provides"]): service.Spec.ExternalName,
	}
	for _, origDeployment := range *deployments {
		deployment := deploymentpatch.New(origDeployment)
		deployment.AppendToPodEnvironment(relationConfig)
		// TODO: The next line is only for benchmarking, remove
		// after benchmarks are finished.
		deployment.AppendToPodLabels(relationConfig)
		patch, err := deployment.GetPatchBytes()
		if err != nil {
			ctxLog.Errorf("Patching failed, cannot encode patch %v", err)
			continue
		}
		if len(patch) == 0 {
			ctxLog.Infof("Nothing to patch..")
			continue
		}
		ctxLog.WithField("patch", string(patch)).Infof("Patching deployment..")
		_, err = t.clientset.AppsV1().Deployments("k8s-tengu-test").Patch(
			origDeployment.Name,
			types.JSONPatchType,
			patch,
		)
		if err != nil {
			ctxLog.Errorf("Patching deployment failed: %v", err)
		} else {
			ctxLog.Infof("Patching deployment succeeded")
		}
	}
}

// ServiceCreated is called when a service is created
func (t *TestHandler) ServiceCreated(obj interface{}) {
	// assert the type to a Service object to pull out relevant data
	service := obj.(*core_v1.Service)
	ctxLog := log.WithFields(log.Fields{
		// '-' prefix is here so these fields are shown first in output
		"-name-watched":            service.Name,
		"-type-watched":            "Service",
		"-resourceVersion-watched": service.ResourceVersion,
	})
	ctxLog.Info("TestHandler.ServiceCreated")

	ctxLog.WithField("ExternalName", service.Spec.ExternalName).Infof("")

	deployments := orconlib.GetRelatedDeploymentsAnnotations(service.Name, t.clientset)
	ctxLog.Infof("Found %v related deployments.", len(*deployments))
	t.addBaseURL(service, deployments, ctxLog)
}

// DeploymentCreated is called when an deployment is created
func (t *TestHandler) DeploymentCreated(obj interface{}) {
	// assert the type to a Service object to pull out relevant data
	deployment := obj.(*appsv1.Deployment)
	ctxLog := log.WithFields(log.Fields{
		// '-' prefix is here so these fields are shown first in output
		"-name-watched":            deployment.Name,
		"-type-watched":            "Deployment",
		"-resourceVersion-watched": deployment.ResourceVersion,
	})
	ctxLog.Infof("TestHandler.DeploymentCreated")

	// servicename := deployment.Labels["tengu.io/relations"]
	// if servicename == "" {
	// 	ctxLog.Infof("Deployment has no relationships.")
	// 	return
	// }
	// service, err := t.clientset.CoreV1().Services("k8s-tengu-test").Get(servicename, metav1.GetOptions{})
	// if err != nil {
	// 	ctxLog.Warnf("Couldn't get service %v: %v", servicename, err)
	// 	return
	// }
	// deployments := []appsv1.Deployment{*deployment}
	// t.addBaseURL(service, &deployments, ctxLog)

	serviceNames := deployment.Annotations["tengu.io/relations"]
	if serviceNames == "" {
		ctxLog.Infof("Deployment has no relationships.")
		return
	}
	for _, serviceName := range strings.Split(serviceNames, ",") {
		service, err := t.clientset.CoreV1().Services("k8s-tengu-test").Get(serviceName, metav1.GetOptions{})
		if err != nil {
			ctxLog.Warnf("Couldn't get service %v: %v", serviceName, err)
			return
		}
		deployments := []appsv1.Deployment{*deployment}
		// this won't work with multiple relations. They all try to change the same environment variable
		// TODO: change names of environment variabels based on consumes/provides relationship
		t.addBaseURL(service, &deployments, ctxLog)
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
