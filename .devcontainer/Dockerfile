FROM mcr.microsoft.com/devcontainers/go:1-bookworm

# Defining versions
ARG KIND_VERSION="v0.31.0"
ARG KUBEBUILDER_VERSION="v4.14.0"
ARG OPERATOR_SDK_VERSION="v1.42.2"
ARG GINKGO_VERSION="v2.28.3"
ARG KUBECTL_VERSION="v1.36.0"

USER root

# Install packages
RUN curl -Lo ./kind https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-amd64 \
    && chmod +x ./kind \
    && mv ./kind /usr/local/bin/kind

RUN curl -L -o kubebuilder "https://github.com/kubernetes-sigs/kubebuilder/releases/download/${KUBEBUILDER_VERSION}/kubebuilder_linux_amd64" \
    && chmod +x kubebuilder \
    && mv kubebuilder /usr/local/bin/

RUN curl -LO "https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk_linux_amd64" \
    && chmod +x operator-sdk_linux_amd64 \
    && mv operator-sdk_linux_amd64 /usr/local/bin/operator-sdk

    # Install kubectl
RUN curl -LO "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" \
    && chmod +x kubectl \
    && mv kubectl /usr/local/bin/

USER vscode

RUN go install github.com/onsi/ginkgo/v2/ginkgo@${GINKGO_VERSION}