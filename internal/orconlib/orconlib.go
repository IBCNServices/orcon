package orconlib

import (
	log "github.com/Sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

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
