package orconlib

import (
	"strings"

	log "github.com/Sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// GetRelatedDeployments returns the deployments related to the resource with given name.
func GetRelatedDeployments(name string, clientset kubernetes.Interface) *[]appsv1.Deployment {
	deploymentList, err := clientset.AppsV1().Deployments("k8s-tengu-test").List(metav1.ListOptions{
		// https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#ListOptions
		LabelSelector: "tengu.io/relations=" + name,
	})
	if err != nil {
		log.Warnf("getting related deployments failed: %v", err)
		return nil
	}
	return &deploymentList.Items
}

// GetRelatedDeploymentsAnnotations return the deployments related to the resource with given name
// expect the list of relations to be small, so linear search is OK
func GetRelatedDeploymentsAnnotations(name string, clientset kubernetes.Interface) *[]appsv1.Deployment {
	deploymentList, err := clientset.AppsV1().Deployments("k8s-tengu-test").List(metav1.ListOptions{
		LabelSelector: "tengu.io/relations",
	})
	if err != nil {
		log.Warnf("getting related deployments failed: %v", err)
		return nil
	}
	var deployments []appsv1.Deployment
	for _, deployment := range deploymentList.Items {
		if relations, ok := deployment.ObjectMeta.Annotations["tengu.io/relations"]; ok {
			for _, relation := range strings.Split(relations, ",") {
				if relation == name {
					deployments = append(deployments, deployment)
				}
			}
		} else {
			log.Warnf("Annotation \"%s\" not present on deployment \"%s\"", "tengu.io/relations", name)
		}
	}

	return &deployments
}
