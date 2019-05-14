package deploymentpatch

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// DeploymentPatch is used to modify a Deployment resource using jsonpatch
type DeploymentPatch struct {
	deployment appsv1.Deployment
	patchList  []patchOperation

	// Internal vars
	labelsEnsured            bool
	podLabelsEnsured         bool
	annotationsEnsured       bool
	podAnnotationsEnsured    bool
	podEnvironmentEnsured    bool
	podInitContainersEnsured bool
}

type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

// New creates a new DeploymentPatch object
func New(deployment appsv1.Deployment) *DeploymentPatch {
	pd := &DeploymentPatch{
		deployment: deployment,
	}
	return pd
}

func (d *DeploymentPatch) ensureLabelsExist() {
	if !d.labelsEnsured {
		if len(d.deployment.Labels) == 0 {
			d.patchList = append(d.patchList, patchOperation{
				Op:    "add",
				Path:  "/metadata/labels",
				Value: struct{}{},
			})
		}
		d.labelsEnsured = true
	}
}

func (d *DeploymentPatch) ensurePodLabelsExist() {
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

func (d *DeploymentPatch) ensureAnnotationsExist() {
	if !d.annotationsEnsured {
		if len(d.deployment.Annotations) == 0 {
			d.patchList = append(d.patchList, patchOperation{
				Op:    "add",
				Path:  "/metadata/annotations",
				Value: struct{}{},
			})
		}
		d.annotationsEnsured = true
	}
}

func (d *DeploymentPatch) ensurePodAnnotationsExist() {
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

func (d *DeploymentPatch) ensurePodEnvironmentExists() {
	if !d.podEnvironmentEnsured {
		for index := range d.deployment.Spec.Template.Spec.Containers {
			if len(d.deployment.Spec.Template.Spec.Containers[index].Env) == 0 {
				d.patchList = append(d.patchList, patchOperation{
					Op:    "add",
					Path:  "/spec/template/spec/containers/" + strconv.Itoa(index) + "/env",
					Value: []struct{}{},
				})
			}
		}
		d.podEnvironmentEnsured = true
	}
}

func (d *DeploymentPatch) ensurePodInitContainersExists() {
	if !d.podInitContainersEnsured {
		if len(d.deployment.Spec.Template.Spec.InitContainers) == 0 {
			d.patchList = append(d.patchList, patchOperation{
				Op:    "add",
				Path:  "/spec/template/spec/initContainers",
				Value: []struct{}{},
			})
		}
		d.podInitContainersEnsured = true
	}
}

// AppendToLabels appends the given map of labels to the deployment
func (d *DeploymentPatch) AppendToLabels(config map[string]string) {
	d.ensureLabelsExist()
	for key, value := range config {
		if d.deployment.Labels[key] == value {
			// Already set; nothing to do here.
			continue
		}
		// https://stackoverflow.com/questions/36147137/kubernetes-api-add-label-to-pod#comment98654379_36163917
		escapedKey := strings.Replace(key, "~", "~0", -1)
		escapedKey = strings.Replace(escapedKey, "/", "~1", -1)
		d.patchList = append(d.patchList, patchOperation{
			Op:    "add",
			Path:  "/metadata/labels/" + escapedKey,
			Value: value,
		})
	}
}

// AppendToPodLabels appends the given map of labels to the pod template
func (d *DeploymentPatch) AppendToPodLabels(config map[string]string) {
	d.ensurePodLabelsExist()
	for key, value := range config {
		if d.deployment.Spec.Template.Labels[key] == value {
			// Already set; nothing to do here.
			continue
		}
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

// AppendToAnnotations adds given map of annotations to the deployment
func (d *DeploymentPatch) AppendToAnnotations(config map[string]string) {
	d.ensureAnnotationsExist()
	for key, value := range config {
		// https://stackoverflow.com/questions/36147137/kubernetes-api-add-label-to-pod#comment98654379_36163917
		escapedKey := strings.Replace(key, "~", "~0", -1)
		escapedKey = strings.Replace(escapedKey, "/", "~1", -1)
		d.patchList = append(d.patchList, patchOperation{
			Op:    "add",
			Path:  "/metadata/annotations/" + escapedKey,
			Value: value,
		})
	}
}

// AppendToPodAnnotations adds given map of annotations to the pod template
func (d *DeploymentPatch) AppendToPodAnnotations(config map[string]string) {
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

// getKeyIdx gets the index of the given key in the env vars.
func getKeyIdx(key string, env []corev1.EnvVar) int {
	for index, value := range env {
		if value.Name == key {
			return index
		}
	}
	return -1
}

// AppendToPodEnvironment adds the map of environment variables to all containers
// and initContainers that are in the original podspec
func (d *DeploymentPatch) AppendToPodEnvironment(config map[string]string) {
	d.ensurePodEnvironmentExists()

	for index := range d.deployment.Spec.Template.Spec.Containers {
		for key, value := range config {
			// Key exists in environment; modifying it.
			existingIdx := getKeyIdx(key, d.deployment.Spec.Template.Spec.Containers[index].Env)
			if existingIdx >= 0 {
				if d.deployment.Spec.Template.Spec.Containers[index].Env[existingIdx].Value == value {
					// Already set, skipping.
					continue
				}
				d.patchList = append(d.patchList, patchOperation{
					Op:   "replace",
					Path: fmt.Sprintf("/spec/template/spec/containers/%v/env/%v", strconv.Itoa(index), strconv.Itoa(existingIdx)),
					Value: map[string]string{
						"name":  key,
						"value": value,
					},
				})
			} else {
				// Key doesn't exist in environment; adding it.
				d.patchList = append(d.patchList, patchOperation{
					Op:   "add",
					Path: "/spec/template/spec/containers/" + strconv.Itoa(index) + "/env/-",
					Value: map[string]string{
						"name":  key,
						"value": value,
					},
				})
			}
		}
	}
	for index := range d.deployment.Spec.Template.Spec.InitContainers {
		for key, value := range config {
			existingIdx := getKeyIdx(key, d.deployment.Spec.Template.Spec.InitContainers[index].Env)
			if existingIdx >= 0 {
				if d.deployment.Spec.Template.Spec.InitContainers[index].Env[existingIdx].Value == value {
					// Already set, skipping.
					continue
				}
				d.patchList = append(d.patchList, patchOperation{
					Op:   "replace",
					Path: fmt.Sprintf("/spec/template/spec/initContainers/%v/env/%v", strconv.Itoa(index), strconv.Itoa(existingIdx)),
					Value: map[string]string{
						"name":  key,
						"value": value,
					},
				})
			} else {
				d.patchList = append(d.patchList, patchOperation{
					Op:   "add",
					Path: "/spec/template/spec/initContainers/" + strconv.Itoa(index) + "/env/-",
					Value: map[string]string{
						"name":  key,
						"value": value,
					},
				})
			}
		}
	}
}

// PrependToPodInitContainers prepends an init container to the template
func (d *DeploymentPatch) PrependToPodInitContainers(container corev1.Container) {
	d.ensurePodInitContainersExists()

	d.patchList = append(d.patchList, patchOperation{
		Op:    "add",
		Path:  "/spec/template/spec/initContainers/0",
		Value: container,
	})
}

// GetPatchBytes returns the resulting jsonPatch
func (d *DeploymentPatch) GetPatchBytes() ([]byte, error) {
	if len(d.patchList) > 0 {
		return json.Marshal(d.patchList)
	}
	return []byte{}, nil
}
