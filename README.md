# Home Assignment

When operating a large multi-tenant Kubernetes cluster, tenants are usually isolated by Namespaces and Role Base Access Control (RBAC).
This approach limits the permissions tenant have on the Namespace object they use to deploy their applications.
Some tenants would like to set specific labels on their Namespace; however, they cannot edit it.
As operators, we came up with the idea of creating a Custom Resource Definition (CRD), which will allow tenants to edit their
Namespace's labels.

**Please make sure you have a basic understanding of the following concepts before you continue to read.**
- [Controller](https://kubernetes.io/docs/concepts/architecture/controller/) 
- [Custom Resource Definition (CRD)](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)
- [Operator](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) 
- [Kubebuilder](https://book.kubebuilder.io)
- [Operator-SDK](https://sdk.operatorframework.io/docs/)

## NamespaceLabel Operator

This operator should be reasonably straightforward. It should sync between the NamespaceLabel CRD and the Namespace Labels.
Various ways could achieve this functionality. Please go ahead and get creative. However, even a simple working solution is good.

An example of a NamespaceLabel CR: 

```
apiVersion: namespacelabel.dana.io/v1alpha1
kind: NamespaceLabel
metadata:
    name: namespacelabel-sample
    namespace: default
spec:
    labels:
        label_1: a
        label_2: b
        label_3: c
```

### Things to address

- Can you create/update/delete labels?
- Can you deal with more than one NamespaceLabel object per Namespace? If not, solve it.
- Namespaces usually have [labels for management](https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/), can you protect those labels?
- Tenant is not able to consume CRDs by default, what needs to be done to let tenant use the NamespaceLabel CRD?
- Code should be documented, tested (unit testing) and well-written.

## Tools you should use
This repo contains a go project you can fork it and use it as a template, also you will need:
- [Kind](https://kind.sigs.k8s.io)  for creating local cluster
- [Go](https://go.dev) your operator should be written in Go
- [Kubebuilder](https://book.kubebuilder.io) for creating the operator and crd template
- [Operator-SDK](https://sdk.operatorframework.io/docs/) for documentation about controllers and syntax
- [Ginkgo](https://onsi.github.io/ginkgo/) for testing

## Bonus
- Use GitHub actions to protect the main branch and test every pull request automatically
- Implement e2e testing
- [Use ECS for logging](https://www.elastic.co/guide/en/ecs/current/index.html)
