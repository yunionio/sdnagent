BINDIR:=_output/bin

GO_BUILD_FLAGS:=-mod vendor
GO_BUILD:=go build $(GO_BUILD_FLAGS)
GO_TEST:=go test $(GO_BUILD_FLAGS)
export GO111MODULE:=on

sdnagent:=$(BINDIR)/sdnagent
sdncli:=$(BINDIR)/sdncli
bins:= \
	$(sdnagent) \
	$(sdncli)

all: $(bins)

$(bins): | $(BINDIR)

$(BINDIR):
	mkdir -p $(BINDIR)

$(sdncli):
	$(GO_BUILD) -o $(BINDIR)/sdnagent yunion.io/x/sdnagent/cmd/sdnagent

$(sdnagent):
	$(GO_BUILD) -o $(BINDIR)/sdncli yunion.io/x/sdnagent/cmd/sdncli

proto-gen: pkg/agent/proto/agent.pb.go

pkg/agent/proto/agent.pb.go: pkg/agent/proto/agent.proto
	protoc -I pkg/agent/proto pkg/agent/proto/agent.proto --go_out=plugins=grpc:pkg/agent/proto

pkg/agent/proto/agent_pb2.py: pkg/agent/proto/agent.proto
	python -m grpc_tools.protoc -Ipkg/agent/proto --python_out=pkg/agent/proto --grpc_python_out=pkg/agent/proto pkg/agent/proto/agent.proto

RELEASE_BRANCH:=master

GOPROXY ?= direct

mod:
	GOPROXY=$(GOPROXY) GONOSUMDB=yunion.io/x go get yunion.io/x/onecloud@$(RELEASE_BRANCH) yunion.io/x/cloudmux@$(RELEASE_BRANCH)
	GOPROXY=$(GOPROXY) GONOSUMDB=yunion.io/x go get $(MAKE_MODE_ARGS) $(patsubst %,%@master,$(shell GO111MODULE=on go mod edit -print | sed -n -e 's|.*\(yunion.io/x/[a-z].*\) v.*|\1|p' | grep -v '/onecloud$$' | grep -v '/cloudmux$$' | grep -v 'openvswitch$$' ))
	GOPROXY=$(GOPROXY) GONOSUMDB=yunion.io/x go mod tidy
	GOPROXY=$(GOPROXY) GONOSUMDB=yunion.io/x go mod vendor -v

.PHONY: mod

test:
	$(GO_TEST)  -v ./...

rpm: $(bins)
	EXTRA_BINS=sdncli \
		 $(CURDIR)/build/build.sh sdnagent

REGISTRY ?= registry.cn-beijing.aliyuncs.com/yunionio
VERSION ?= $(shell git describe --exact-match 2> /dev/null || \
	   git describe --match=$(git rev-parse --short=8 HEAD) --always --dirty --abbrev=8)
IMAGE_NAME_TAG := $(REGISTRY)/sdnagent:$(VERSION)

DOCKER_ALPINE_BUILD_IMAGE:=registry.cn-beijing.aliyuncs.com/yunionio/alpine-build:3.22.0-go-1.24.6-0
docker-alpine-build:
	docker run --rm \
		--name "docker-alpine-build-onecloud-sdnagent" \
		-v $(CURDIR):/root/go/src/yunion.io/x/sdnagent \
		-v $(CURDIR)/_output/alpine-build:/root/go/src/yunion.io/x/sdnagent/_output \
		$(DOCKER_ALPINE_BUILD_IMAGE) \
		/bin/sh -c "set -ex; cd /root/go/src/yunion.io/x/sdnagent; make $(F); chown -R $$(id -u):$$(id -g) _output"

docker-alpine-build-stop:
	docker stop --time 0 docker-alpine-build-onecloud-sdnagent || true
.PHONY: docker-alpine-build
.PHONY: docker-alpine-build-stop

docker-image:
	DEBUG=${DEBUG} REGISTRY=${REGISTRY} TAG=${VERSION} ARCH=${ARCH} ${CURDIR}/scripts/docker_push.sh

docker-image-push:
	PUSH=true DEBUG=${DEBUG} REGISTRY=${REGISTRY} TAG=${VERSION} ARCH=${ARCH} ${CURDIR}/scripts/docker_push.sh

image: docker-image-push

.PHONY: docker-image
.PHONY: docker-image-push
.PHONY: image

.PHONY: all $(bins) rpm test
