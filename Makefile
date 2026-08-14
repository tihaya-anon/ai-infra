CLUSTER ?= ai-infra-lab
IMAGE ?= ai-infra-lab:dev
RUNTIME_IMAGE ?= m.daocloud.io/gcr.io/distroless/static-debian12:nonroot
JOBSET_VERSION ?= v0.10.1
KUEUE_VERSION ?= v0.14.3
GOIMPORTS := ./scripts/goimports.sh

.PHONY: tools fmt fmt-check line-length vet test verify hooks build image cluster deploy demo clean

tools:
	$(GOIMPORTS) --install

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

verify: fmt-check line-length vet test

hooks: tools
	pre-commit install

build:
	go build ./cmd/controller ./cmd/scheduler ./cmd/worker

image:
	docker build --build-arg RUNTIME_IMAGE=$(RUNTIME_IMAGE) -t $(IMAGE) .

cluster:
	kind create cluster --name $(CLUSTER) --config kind.yaml

deploy: image
	kind load docker-image $(IMAGE) --name $(CLUSTER)
	kubectl apply --server-side -f https://github.com/kubernetes-sigs/jobset/releases/download/$(JOBSET_VERSION)/manifests.yaml
	kubectl -n jobset-system rollout status deployment/jobset-controller-manager --timeout=180s
	kubectl apply --server-side -f https://github.com/kubernetes-sigs/kueue/releases/download/$(KUEUE_VERSION)/manifests.yaml
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

clean:
	kind delete cluster --name $(CLUSTER)
