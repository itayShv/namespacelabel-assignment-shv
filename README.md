# NamespaceLabel Kubernetes Operator

## Overview
In large, multi-tenant Kubernetes clusters, tenant isolation is strictly enforced via Namespaces and Role-Based Access Control (RBAC). While this prevents unauthorized access, it also prevents tenants from modifying their own Namespace objects to add custom operational labels.

The **NamespaceLabel Operator** solves this by providing a controlled, secure Custom Resource Definition (CRD). It acts as a secure proxy, allowing tenants to manage their Namespace labels via a declarative `NamespaceLabel` CR without requiring native `patch` or `update` permissions on the core Namespace object itself.

---

## Core Capabilities & Architectural Design

This operator was designed with enterprise-grade security, performance, and lifecycle management in mind:

* **Complete Lifecycle Management (CRUD):** Continuously syncs the state of the `NamespaceLabel` CR to the target Namespace. Supports creating, updating, and completely deleting managed labels.
* **State Collision Prevention (The Singleton Pattern):** To solve the issue of multiple CRs fighting over the same Namespace, the operator enforces a strict Singleton pattern. It rejects any Custom Resource that is not named exactly `labels`, preventing state drift and etcd bloat.
* **Protected Label Guardrails:** Dynamically fetches restricted prefixes from a central `protected-labels` ConfigMap. It actively prevents tenants from overwriting or deleting critical cluster-management labels (e.g., `kubernetes.io/`, `k8s.io/`).
* **Native Tenant RBAC (Aggregated Roles):** By default, tenants cannot consume new CRDs. This operator utilizes Kubernetes **Aggregated ClusterRoles** (`aggregate-to-edit`, `aggregate-to-admin`). If a tenant has standard `edit` permissions for a namespace, the API Server dynamically merges the CRD permissions, allowing them to manage labels seamlessly without custom RoleBindings.
* **Zero Privilege Escalation (Self-Targeting):** The Reconcile loop enforces a strict constraint: A `NamespaceLabel` CR can only modify the exact Namespace in which it resides, completely eliminating the risk of a tenant labeling out-of-scope production namespaces.
* **High-Performance Drift Detection:** Features an optimized custom event predicate. The operator silently drops events from unmanaged namespaces, ensuring zero CPU spikes even in clusters with tens of thousands of namespaces and background noise.
* **Strict Garbage Collection:** Implements robust Finalizer logic. When a tenant deletes the CR, the operator gracefully cleans up all previously managed labels on the Namespace before allowing Kubernetes to physically remove the object from etcd.

---

## Prerequisites
* [Go](https://go.dev/) (v1.22+)
* [Docker](https://docs.docker.com/get-docker/)
* [Kind](https://kind.sigs.k8s.io/)
* [Kubebuilder](https://book.kubebuilder.io/)
* [Ginkgo](https://onsi.github.io/ginkgo/)

---

## Development Environment (DevContainer)

This project includes a fully configured VSCode DevContainer to ensure a consistent, reproducible development environment. Reviewers or contributors can spin up the entire toolchain in seconds without polluting their local machine.

### Features Included
* **Docker-in-Docker:** Run `kind` and build container images directly from within the workspace.
* **Pre-installed Toolchain:** Go, Kind, Kubebuilder, Operator-SDK, kubectl, and Ginkgo are pre-compiled and ready to use.
* **VSCode Extensions:** Pre-configured with the Go language server and Kubernetes tools.

### How to Start
1. Ensure you have Docker and VSCode installed on your machine.
2. Install the **Dev Containers** extension in VSCode (`ms-vscode-remote.remote-containers`).
3. Clone this repository and open the folder in VSCode.
4. VSCode will prompt you to **"Reopen in Container"**. Click it! (Alternatively, open the Command Palette `F1` and select `Dev Containers: Reopen in Container`).

Once the container builds, you can open a terminal inside VSCode and immediately run `make test` or `kind create cluster`—everything is already configured for you!

---

## Getting Started

### Option A: Automated Deployment Script (`deploy.sh`)

You can use the included bash script to automate the entire build, load, and deploy process into a local Kind cluster. 

Save the following as `deploy.sh` in the root of the project, run `chmod +x deploy.sh`, and execute it:

```bash
#!/bin/bash
set -e

# Configuration
CLUSTER_NAME="operator-sandbox"
IMG="namespacelabel-controller:dev"
NAMESPACE="namespacelabel-assignment-shv-system"

echo "🚀 Starting Deployment Process..."

# 1. Create Kind cluster if it doesn't exist
if ! kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
  echo "📦 Creating Kind cluster '${CLUSTER_NAME}'..."
  kind create cluster --name ${CLUSTER_NAME}
else
  echo "✅ Kind cluster '${CLUSTER_NAME}' already exists."
fi

# 2. Build the Docker Image
echo "🔨 Building Docker image: ${IMG}..."
make docker-build IMG=${IMG}

# 3. Load Image into Kind
echo "📥 Loading image into Kind cluster..."
kind load docker-image ${IMG} --name ${CLUSTER_NAME}

# 4. Deploy CRDs, RBAC, and Controller
echo "🚀 Deploying operator to the cluster..."
make deploy IMG=${IMG}

echo "✅ Deployment complete! Waiting for pods to spin up..."
sleep 5
kubectl get pods -n ${NAMESPACE}

```

### Option B: Run Locally for Quick Debugging

To run the controller locally against your active Kubernetes cluster context (bypassing the need to build a Docker image every time):

```bash
make install
make run

```

---

## Usage Guide

### 1. Configure Protected Labels (Admin)

By default, `kubernetes.io/` and `k8s.io/` are protected. A cluster admin can add custom protected prefixes by deploying a ConfigMap named `protected-labels` in the `default` namespace:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: protected-labels
  namespace: default
data:
  protected-prefixes: "kubernetes.io/,k8s.io/,itays.protected/"

```

### 2. Apply Tenant Labels (Tenant)

Once the operator is running, apply a `NamespaceLabel` CR into your desired namespace.
*Note: To satisfy the Singleton pattern, the CR must be named exactly `labels`.*

```yaml
apiVersion: namespacelabel.dana.exam/v1alpha1
kind: NamespaceLabel
metadata:
  name: labels
  namespace: default
spec:
  labels:
    tenant: my-team
    environment: staging

```

Apply the resource and verify:

```bash
kubectl apply -f config/samples/namespacelabel_v1alpha1_namespacelabel.yaml
kubectl get namespace default --show-labels

```

---

## RBAC Verification Guide

To prove the security and RBAC Aggregation mechanisms work, you can use Kubernetes impersonation to verify tenant permissions.

**1. Create a test namespace:**

```bash
kubectl create ns rbac-test

```

**2. Verify default deny (an unprivileged user has no access):**

```bash
kubectl auth can-i create namespacelabels -n rbac-test --as=test-dev
# Expected Output: no

```

**3. Grant the user standard 'edit' rights to the namespace:**

```bash
kubectl create rolebinding test-dev-edit-binding \
  --clusterrole=edit \
  --user=test-dev \
  --namespace=rbac-test

```

**4. Verify the Aggregated Role took effect (user can now manage the CR):**

```bash
kubectl auth can-i create namespacelabels -n rbac-test --as=test-dev
# Expected Output: yes

```

**5. Verify the boundary (user cannot access other namespaces):**

```bash
kubectl auth can-i create namespacelabels -n default --as=test-dev
# Expected Output: no

```

---

## Testing

The codebase is thoroughly documented and unit-tested using the `envtest` framework to simulate a real Kubernetes control plane in memory.

To run the unit testing suite, execute:

```bash
make test

```

---

## Cleanup & Teardown

To completely remove the operator, its CRDs, and its RBAC permissions from the cluster, run:

```bash
make undeploy

```

```

```