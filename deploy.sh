# Configuration
CLUSTER_NAME="operator-sandbox"
IMG="namespacelabel-controller:dev"
NAMESPACE="namespacelabel-assignment-shv-system"

# 1. Create Kind cluster if it doesn't exist
if ! kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
  echo "📦 Creating Kind cluster '${CLUSTER_NAME}'..."
  kind create cluster --name ${CLUSTER_NAME}
else
  echo "Kind cluster '${CLUSTER_NAME}' already exists."
fi

#Install Cert-Manager (Required for Webhook) only if missing
if ! kubectl get deployment cert-manager -n cert-manager >/dev/null 2>&1; then
  echo "Installing Cert-Manager for Webhook TLS..."
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.yaml
  
  echo "Waiting for Cert-Manager Webhook to become available..."
  # This waits intelligently instead of using a hardcoded sleep
  kubectl wait --for=condition=Available deployment/cert-manager-webhook -n cert-manager --timeout=120s
else
  echo "✅ Cert-Manager is already installed."
fi


# 2. Build the Docker Image
echo "Building Docker image: ${IMG}..."
make docker-build IMG=${IMG}

# 3. Load Image into Kind
echo "Loading image into Kind cluster..."
kind load docker-image ${IMG} --name ${CLUSTER_NAME}

# 4. Deploy CRDs, RBAC, and Controller
echo "Deploying operator to the cluster..."
make deploy IMG=${IMG}
