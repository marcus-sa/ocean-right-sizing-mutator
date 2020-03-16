VERSION?=$$(cat VERSION)
IMAGE?=ocean-rs-mutator
IMAGE_REPOSITORY?=spotinst
IMAGE_TAG?=$(VERSION)
K8S_SVC_NAME?=ocean-rs-mutator-svc
K8S_NS_NAME?=kube-system
K8S_SECRET_NAME?=ocean-rs-mutator-secret
V?=5
V-Deployment?=5


all: gencerts-deploy deploy build build-release push clean

.PHONY: runlocal
runlocal: ## Build and run - localy
	go build -o dist/$(IMAGE) &&  SPOTINST_CONTROLLER_ID=${SPOTINST_CONTROLLER_ID} SPOTINST_TOKEN=${SPOTINST_TOKEN} SPOTINST_ACCOUNT=${SPOTINST_ACCOUNT} ./dist/$(IMAGE) -tls-cert-file tls/server-cert.pem -tls-private-key-file tls/server-key.pem -v $(V)

.PHONY: gencerts-deploy
gencerts-deploy: ## Generate certificates and apply them to the current cluster
	./gen_certs_self_signed.sh --service $(K8S_SVC_NAME) --namespace kube-system --secret $(K8S_SECRET_NAME) --verbosity $(V-Deployment)

.PHONY: deploy
deploy: ## Deploy the mutator (Deployment/Service/MutatingWebhook)
	kubectl apply -f deployment/

.PHONY: undeploy
undeploy: ## Delete all K8s objects from the current cluster and delete all generated K8s yamls
	kubectl delete -f deployment/

.PHONY: delcerts
delcerts: ## Delete all from deployment and tls directories
	@rm -f deployment/*.yaml
	@rm -rf tls

.PHONY: build
build: ## Build for loacl deployment
	go build -o dist/$(IMAGE) 

.PHONY: build-release
build-release: ## build linux binary and docker image
	GOOS=linux GOARCH=amd64 go build -o dist/$(IMAGE)-linux
	docker build -t $(IMAGE_REPOSITORY)/$(IMAGE):$(IMAGE_TAG) . 

.PHONY: push
push:
	docker push $(IMAGE_REPOSITORY)/$(IMAGE):$(IMAGE_TAG)

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)