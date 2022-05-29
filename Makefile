SHELL := /bin/bash

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# options for generating crds with controller-gen
CONTROLLER_GEN="${GOBIN}/controller-gen"
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

.EXPORT_ALL_VARIABLES:

default: build

fmt: ## Run go fmt against code.
	go fmt ./cmd/... ./pkg/...

vet: ## Run go vet against code.
	go vet ./cmd/... ./pkg/...

test: fmt vet envtest ## Run tests.
	go test -v ./pkg/...

e2etest: fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -v ./test/...

build:
	go build -o out/jvmbuildservice cmd/controller/main.go
	env GOOS=linux GOARCH=amd64 go build -mod=vendor -o out/jvmbuildservice ./cmd/controller

clean:
	rm -rf out

generate-deepcopy-client:
	hack/update-codegen.sh

generate-crds:
	hack/install-controller-gen.sh
	"$(CONTROLLER_GEN)" "$(CRD_OPTIONS)" rbac:roleName=manager-role webhook paths=./pkg/apis/jvmbuildservice/v1alpha1 output:crd:artifacts:config=deploy/crds/base

generate: generate-crds generate-deepcopy-client

verify-generate-deepcopy-client: generate-deepcopy-client
	hack/verify-codegen.sh

dev-image-cache:
	docker build java-components -f java-components/cache/src/main/docker/Dockerfile -t quay.io/$(QUAY_USERNAME)/hacbs-jvm-cache:dev
	docker push quay.io/$(QUAY_USERNAME)/hacbs-jvm-cache:dev

dev-image-sidecar:
	docker build java-components -f java-components/sidecar/src/main/docker/ -t quay.io/$(QUAY_USERNAME)/hacbs-jvm-sidecar:dev
	docker push quay.io/$(QUAY_USERNAME)/hacbs-jvm-sidecar:dev

dev-image-build-request-processor:
	docker build java-components -f java-components/build-request-processor/src/main/docker/Dockerfile -t quay.io/$(QUAY_USERNAME)/hacbs-jvm-build-request-processor:dev
	docker push quay.io/$(QUAY_USERNAME)/hacbs-jvm-build-request-processor:dev

dev-image-dependency-analyser:
	docker build java-components -f java-components/dependency-analyser/src/main/docker/Dockerfile -t quay.io/$(QUAY_USERNAME)/hacbs-jvm-dependency-analyser:dev
	docker push quay.io/$(QUAY_USERNAME)/hacbs-jvm-dependency-analyser:dev

dev-image-controller:
	docker build . -t quay.io/$(QUAY_USERNAME)/hacbs-jvm-controller:dev
	docker push quay.io/$(QUAY_USERNAME)/hacbs-jvm-controller:dev

dev: dev-image-controller dev-image-cache dev-image-build-request-processor dev-image-dependency-analyser dev-image-sidecar
	./deploy/development.sh

ENVTEST = $(shell pwd)/bin/setup-envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef