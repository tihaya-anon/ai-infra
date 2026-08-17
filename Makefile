CLUSTER ?= ai-infra-lab
IMAGE ?= ai-infra-lab:dev
KIND_NODE_IMAGE ?= m.daocloud.io/docker.io/kindest/node:v1.34.8@sha256:02722c2dedddcfc00febf5d27fbeb9b7b2c14294c82109ff4a85d89ac9ba3256
GO_BUILDER_IMAGE ?= m.daocloud.io/docker.io/library/golang:1.24.0
GOPROXY ?= https://goproxy.cn,direct
RUNTIME_IMAGE ?= m.daocloud.io/gcr.io/distroless/static-debian12:nonroot
EXTERNAL_IMAGE_MIRROR ?= m.daocloud.io
KIND_IMAGE_PLATFORM ?=
HEADLAMP_NAMESPACE ?= kube-system
HEADLAMP_ADMIN_SERVICE_ACCOUNT ?= headlamp-admin
HEADLAMP_PORT ?= 4466
JOBSET_IMAGE ?= registry.k8s.io/jobset/jobset:v0.10.1
KUEUE_IMAGE ?= registry.k8s.io/kueue/kueue:v0.14.3
GOIMPORTS := ./scripts/goimports.sh
CONTROLLER_GEN := ./scripts/controller-gen.sh
ENVTEST_K8S_VERSION ?= 1.34.0

.PHONY: tools generate fmt fmt-check line-length vet test test-api test-e2e verify hooks build image cluster preload-external-images deploy demo headlamp headlamp-token headlamp-port-forward headlamp-port-forward-stop benchmark benchmark-validate failure-capacity failure-worker failure-restart clean

tools:
	$(GOIMPORTS) --install
	$(CONTROLLER_GEN) --install

generate: tools
	$(CONTROLLER_GEN) object paths=./api/...
	@tmp_dir="$$(mktemp -d)"; \
	trap 'rm -rf "$$tmp_dir"' EXIT; \
	$(CONTROLLER_GEN) crd:maxDescLen=0 paths=./api/... output:crd:dir="$$tmp_dir"; \
	cp "$$tmp_dir/infra.example.io_aijobs.yaml" deploy/crd.yaml
	go run ./scripts/sync-aijob-schema.go
	$(GOIMPORTS) -w ./api

fmt:
	$(GOIMPORTS) -w .

fmt-check:
	@unformatted="$$( $(GOIMPORTS) -l . )" || exit $$?; \
	if [ -n "$$unformatted" ]; then \
		echo "Go files need formatting; run 'make fmt':"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

line-length:
	find . -type f -name '*.go' -not -path './.tools/*' \
		-exec ./scripts/check-go-line-length.sh {} +

vet:
	go vet ./...

test:
	go test ./...

test-api:
	KUBEBUILDER_ASSETS="$$(./scripts/setup-envtest.sh $(ENVTEST_K8S_VERSION))" \
	JOBSET_CRD_PATH="$$(go list -m -f '{{.Dir}}' sigs.k8s.io/jobset)/config/components/crd/bases/jobset.x-k8s.io_jobsets.yaml" \
	go test -tags=api_test ./internal/controller \
		-run '^TestGivenAPIEnvironmentWhenReconcilingAIJobThenResourcesAndStatusAreProjected$$' \
		-count=1

test-e2e:
	go run ./cmd/labctl e2e --cluster $(CLUSTER)

verify: fmt-check line-length vet test

hooks: tools
	pre-commit install

build:
	go build ./cmd/controller ./cmd/scheduler ./cmd/worker ./cmd/labctl

image:
	docker build --build-arg GO_BUILDER_IMAGE=$(GO_BUILDER_IMAGE) --build-arg GOPROXY=$(GOPROXY) --build-arg RUNTIME_IMAGE=$(RUNTIME_IMAGE) -t $(IMAGE) .

cluster:
	kind create cluster --name $(CLUSTER) --image $(KIND_NODE_IMAGE) --config kind.yaml

preload-external-images:
	CLUSTER=$(CLUSTER) KIND_IMAGE_PLATFORM=$(KIND_IMAGE_PLATFORM) KIND_IMAGE_MIRROR_PREFIX=$(EXTERNAL_IMAGE_MIRROR) ./scripts/load-kind-images.sh $(JOBSET_IMAGE) $(KUEUE_IMAGE)

deploy: image preload-external-images
	kind load docker-image $(IMAGE) --name $(CLUSTER)
	kubectl apply --server-side -k deploy/dependencies/jobset
	kubectl -n jobset-system rollout status deployment/jobset-controller-manager --timeout=180s
	kubectl apply --server-side -k deploy/dependencies/kueue
	kubectl -n kueue-system rollout status deployment/kueue-controller-manager --timeout=180s
	kubectl apply -f deploy/crd.yaml
	kubectl apply -f deploy/rbac.yaml
	kubectl apply -f deploy/kueue-resources.yaml
	kubectl apply -f deploy/controller.yaml
	kubectl apply -f deploy/scheduler-config.yaml
	kubectl -n ai-infra-system rollout status deployment/aijob-controller --timeout=120s
	kubectl -n ai-infra-system rollout status deployment/ai-scheduler --timeout=120s

demo:
	./scripts/label-nodes.sh
	kubectl apply -f examples/aijob.yaml

headlamp:
	CLUSTER=$(CLUSTER) \
	HEADLAMP_NAMESPACE=$(HEADLAMP_NAMESPACE) \
	HEADLAMP_ADMIN_SERVICE_ACCOUNT=$(HEADLAMP_ADMIN_SERVICE_ACCOUNT) \
	./scripts/install-headlamp.sh

headlamp-token:
	@HEADLAMP_NAMESPACE=$(HEADLAMP_NAMESPACE) \
	HEADLAMP_ADMIN_SERVICE_ACCOUNT=$(HEADLAMP_ADMIN_SERVICE_ACCOUNT) \
	./scripts/headlamp-token.sh

headlamp-port-forward:
	@HEADLAMP_NAMESPACE=$(HEADLAMP_NAMESPACE) HEADLAMP_PORT=$(HEADLAMP_PORT) \
	./scripts/headlamp-port-forward.sh start

headlamp-port-forward-stop:
	@HEADLAMP_NAMESPACE=$(HEADLAMP_NAMESPACE) HEADLAMP_PORT=$(HEADLAMP_PORT) \
	./scripts/headlamp-port-forward.sh stop

benchmark:
	go run ./cmd/labctl benchmark --cluster $(CLUSTER)

benchmark-validate:
	go run ./cmd/labctl validate-results --dir out/benchmark

failure-capacity:
	go run ./cmd/labctl exercise --cluster $(CLUSTER) --kind capacity

failure-worker:
	go run ./cmd/labctl exercise --cluster $(CLUSTER) --kind worker-failure

failure-restart:
	go run ./cmd/labctl exercise --cluster $(CLUSTER) --kind controller-restart

clean:
	kind delete cluster --name $(CLUSTER)
