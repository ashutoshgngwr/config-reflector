GOFLAGS := GO111MODULE=on GOOS=linux CGO_ENABLED=0
GO := $(GOFLAGS) go
GOBUILD := $(GO) build -tags netgo -ldflags '-extldflags "-static"' -a

SRC_PATH := ./cmd/manager
BIN_PATH := ./build/_output
OPERATOR_BIN := config-reflector

DOCKER_IMAGE := ashutoshgngwr/config-reflector

ARCH := $(shell uname -m)
OSTYPE := $(shell uname)

GOLANG_LINT_VERSION := v1.17.1
GOLANG_LINT_BIN := golangci-lint
GOLANG_LINT_INSTALL_URL := https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh

all: lint clean build docker ## build clean binaries and docker image

build: ## build operator binary
	@echo "Building operator binary..."
	@$(GOBUILD) -o $(BIN_PATH)/$(OPERATOR_BIN) $(SRC_PATH)
	@strip $(BIN_PATH)/$(OPERATOR_BIN)

clean: ## remove operator binary
	@echo "Removing operator binary..."
	@rm -f $(BIN_PATH)/$(OPERATOR_BIN)

clean-all: ## remove all build files
	@echo "Removing all build files..."
	@rm -rf $(BIN_PATH)

docker: ## build docker image for the operator
	@echo "Building docker image for operator..."
	@docker build -t $(DOCKER_IMAGE):latest -f ./build/Dockerfile .

golangci-lint: mkdir-bin
	@test -f $(BIN_PATH)/$(GOLANG_LINT_BIN) || \
		{ echo "Downloading golangci-lint binary..." ;\
		  curl -sfL $(GOLANG_LINT_INSTALL_URL) | sh -s -- -b $(BIN_PATH) $(GOLANG_LINT_VERSION) ; }

help: ## show this message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

lint: golangci-lint ## perform static analysis on Go code
	@echo "Performing static code analysis..."
	@$(BIN_PATH)/$(GOLANG_LINT_BIN) run --color always **/*.go

mkdir-bin:
	@test -d $(BIN_PATH) || mkdir -p $(BIN_PATH)

run: ## run operator locally
	@WATCH_NAMESPACE="" $(GO) run $(SRC_PATH) --devel

.PHONY: all build clean clean-all docker golangci-lint help lint mkdir-bin run
.DEFAULT_GOAL := help
