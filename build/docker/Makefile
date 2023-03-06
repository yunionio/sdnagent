REGISTRY ?= "registry.cn-beijing.aliyuncs.com/yunionio"
DOCKER_BUILD = docker build -t $(REGISTRY)
DOCKER_BUILDX = docker buildx build --platform linux/arm64,linux/amd64 --push -t $(REGISTRY)


SDNAGENT_BASE_VERSION = v0.0.2
sdnagent-base:
	$(DOCKER_BUILDX)/sdnagent-base:$(SDNAGENT_BASE_VERSION) -f ./Dockerfile.sdnagent-base .
