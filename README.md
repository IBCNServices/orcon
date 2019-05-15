# Orchestrator Conversation

This repository contains a prototype implementation of the Orchestrator Conversation; a service orchestration framework inspired by Juju.

## Prerequisites

Kubernetes 1.9.0 or above with the `admissionregistration.k8s.io/v1beta1` API enabled. Verify that by the following command.

```console
$kubectl api-versions | grep "admissionregistration.k8s.io/v1beta1"
admissionregistration.k8s.io/v1beta1
```

In addition, the `MutatingAdmissionWebhook` and `ValidatingAdmissionWebhook` admission controllers should be added and listed in the correct order in the admission-control flag of kube-apiserver. These are set by default in the CDK.

The Kubernetes cluster needs a certificate signer. Instructions for the CDK bundle are the following:

1. Copy the `ca.key` from the `easyrsa` charm (located in `/var/lib/juju/agents/unit-easyrsa-0/charm/EasyRSA/pki/private`) to all Kubernetes master nodes at `/root/cdk`, permissions `440`.
2. Add the appropriate flags to the `kube-controller` daemon.

   ```bash
   juju config kubernetes-master "controller-manager-extra-args=cluster-signing-cert-file=/root/cdk/ca.crt cluster-signing-key-file=/root/cdk/ca.key"
   ```

## Build

1. Setup dep

   This project uses golang and [dep](https://github.com/golang/dep) as the dependency management tool.

   ```bash
   sudo snap install go --classic
   sudo apt-get install go-dep
   ```

2. Build the code locally

   ```bash
   ./build/builder build relations-controller
   ```

3. Build the code and push the container to dockerhub.

   ```bash
   ./build/builder publish relations-controller
   ```

## Deploy

1. Create a signed cert/key pair and store it in a Kubernetes `secret` that will be consumed by sidecar deployment.

   ```bash
   ./deployment/webhook-create-signed-cert.sh \
       --service relations-mutating-webhook \
       --secret tengu-controllers-certs \
       --namespace default
   ```

2. Patch the `MutatingWebhookConfiguration` by setting `caBundle` with correct value from Kubernetes cluster

   ```bash
   cat deployment/relations-mutating-webhook/webhook-config-templ.yaml | \
       deployment/webhook-patch-ca-bundle.sh > \
       deployment/relations-mutating-webhook/webhook-config-generated.yaml
   ```

3. Deploy resources

   ```bash
   # If RBAC is enabled
   kubectl apply -f deployment/rbac.yaml
   # Deploy the admission controller
   kubectl apply -f deployment/relations-mutating-webhook/controller-configmap.yaml
   kubectl apply -f deployment/relations-mutating-webhook/controller.yaml
   kubectl apply -f deployment/relations-mutating-webhook/service.yaml
   kubectl apply -f deployment/relations-mutating-webhook/webhook-config-generated.yaml
   # Deploy the regular controller
   kubectl apply -f deployment/relations-controller/controller.yaml
   ```

4. Example

   ```bash
   kubectl create namespace k8s-tengu-test
   kubectl label namespace k8s-tengu-test tengu-injector=enabled
   kubectl -n k8s-tengu-test apply -f deployment/demo/external-service.yaml
   kubectl -n k8s-tengu-test apply -f deployment/demo/sleep-deployment.yaml
   ```

## Development

### Develop locally

1. Install [Telepresence](https://www.telepresence.io/) for swapping the k8s service with a proxy that sends requests to your local machine.

2. Install `proot` for simulating the volume mounts on your local machine.

   ```bash
   sudo apt install proot
   ```

3. Start Telepresence

   ```bash
   telepresence --swap-deployment relations-mutating-webhook --expose 8080
   ```

   *Note: Telepresence warns you that vpn-tcp doesn't work with existing vpn's; but it still appears to work with our vpn.*

4. Run script to simulate volume mounts and start Telepresence.

   ```bash
   cd ~/go/src/gitlab.ilabt.imec.be/tengu/orcon/
   ./scripts/simulate-volume-mounts.sh
   ```

5. Build binary from outside of the telepresence environment. You can use the VSCode task `Build relations-mutating-webhook` for this.

6. Run binary inside of Telepresence environment.

   ```bash
   ./bin/relations-mutating-webhook -tenguCfgFile=/etc/webhook/config/tenguconfig.yaml -tlsCertFile=/etc/webhook/certs/cert.pem -tlsKeyFile=/etc/webhook/certs/key.pem -alsologtostderr -v=4
   ```

### Folder Structure

This folder structure is loosely based on the ["Standard Package Layout"](https://medium.com/@benbjohnson/standard-package-layout-7cdbc8391fc1). [Illustrated example](https://medium.com/wtf-dial/wtf-dial-domain-model-9655cd523182) and [more thoughts](https://medium.com/wtf-dial/wtf-dial-re-evaluating-the-domain-32c5ec31b9e2).

This project loosely follows Domain Driven Design. DDD in go [1](https://www.citerus.se/go-ddd), [2](https://www.citerus.se/part-2-domain-driven-design-in-go/), [3](https://www.citerus.se/part-3-domain-driven-design-in-go/).

Golang does not permit circular dependencies. This was initially done to make it easier to write a compiler, but it turned out that it forces projects to really think about their structure and imports.

Working with packages with multiple binaries: <https://ieftimov.com/post/golang-package-multiple-binaries/>
