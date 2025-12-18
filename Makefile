APP_NAME := doris-webhook
APP_VERSION ?= v1
APP_PORT ?= 8080
CONTAINER_COMMAND := $(shell command -v podman 2> /dev/null || command -v docker 2> /dev/null || echo "none")
REGISTRY_DOMAIN ?= docker.io
REGISTRY_PROJECT ?= project
CONTAINER_IMAGE = ${REGISTRY_DOMAIN}/${REGISTRY_PROJECT}/${APP_NAME}:${APP_VERSION}

# check container runtime
ifeq ($(CONTAINER_COMMAND),none)
    $(error "Command <podman> or <docker> not found!")
endif

.PHONY: all build push test
all: build push clean  ## build, push, clean

test: build run clean  ## build, run, clean
	@echo "Testing..."
	@echo $(REGISTRY_DOMAIN)
	@echo $(REGISTRY_PROJECT)

build:  ## build image
	@echo "Building image"
	${CONTAINER_COMMAND} build --build-arg APP_PORT=${APP_PORT} -t ${CONTAINER_IMAGE} .

push:  ## push image
	@echo "Pushing image to registry"
	${CONTAINER_COMMAND} push ${CONTAINER_IMAGE}

clean:  ## clean image
	@echo "Cleaning up"
	${CONTAINER_COMMAND} image prune --force || true;\
	${CONTAINER_COMMAND} rmi ${CONTAINER_IMAGE} --force || true

run:  ## run container
	@echo "Running container"
	${CONTAINER_COMMAND} run -p ${APP_PORT}:${APP_PORT} ${CONTAINER_IMAGE}

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
