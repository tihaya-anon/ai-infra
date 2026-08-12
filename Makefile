CLUSTER ?= ai-infra-lab
IMAGE ?= ai-infra-lab:dev

.PHONY: test build image cluster deploy demo clean

test:
	go test ./...

build:
	go build ./cmd/ai-infra-lab

image:
	docker build -t $(IMAGE) .

cluster:
	kind create cluster --name $(CLUSTER) --config kind.yaml

deploy: image
	kind load docker-image $(IMAGE) --name $(CLUSTER)
	kubectl apply -f deploy/crd.yaml
	kubectl apply -f deploy/rbac.yaml
	kubectl apply -f deploy/deployment.yaml
	kubectl -n ai-infra-system rollout status deployment/ai-infra-lab --timeout=120s

demo:
	./scripts/label-nodes.sh
	kubectl apply -f examples/aijob.yaml

clean:
	kind delete cluster --name $(CLUSTER)
