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

# 2. Build the Docker Image
echo "Building Docker image: ${IMG}..."
make docker-build IMG=${IMG}

# 3. Load Image into Kind
echo "Loading image into Kind cluster..."
kind load docker-image ${IMG} --name ${CLUSTER_NAME}

# 4. Deploy CRDs, RBAC, and Controller
echo "Deploying operator to the cluster..."
make deploy IMG=${IMG}
