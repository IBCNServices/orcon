# Relations MutatingWebhook Admission controller

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

2. Build and push docker image. Replace the docker repo with your own.

   ```bash
   docker login
   ./build galgalesh/tengu-injector:v1
   ```

## Deploy

1. Create a signed cert/key pair and store it in a Kubernetes `secret` that will be consumed by sidecar deployment.

   ```bash
   ./deployment/webhook-create-signed-cert.sh \
       --service sidecar-injector-webhook-svc \
       --secret tengu-injector-webhook-certs \
       --namespace default
   ```

2. Patch the `MutatingWebhookConfiguration` by set `caBundle` with correct value from Kubernetes cluster

   ```bash
   cat deployment/mutatingwebhook.yaml | \
       deployment/webhook-patch-ca-bundle.sh > \
       deployment/mutatingwebhook-ca-bundle.yaml
   ```

3. Deploy resources

   ```bash
   kubectl create -f deployment/configmap.yaml
   kubectl create -f deployment/deployment.yaml
   kubectl create -f deployment/service.yaml
   kubectl create -f deployment/mutatingwebhook-ca-bundle.yaml
   # If RBAC is enabled
   kubectl create -f deployment/rbac.yaml
   ```


4. Example

   ```bash
   kubectl create -f deployment/external-service.yaml
   kubectl create -f deployment/sleep-deployment.yaml
   ```

## TODO list

- Currently the deployments are not restarted when a service becomes available with matching annotations.
- Currently only works in de default namespace.
