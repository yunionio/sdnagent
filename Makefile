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

mod:
	GOPROXY=direct go get -v yunion.io/x/onecloud@master
	GOPROXY=direct go mod vendor -v

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

docker-image:
	docker run --rm \
		-v $(CURDIR):/root/go/src/yunion.io/x/sdnagent \
		-v $(CURDIR)/_output/alpine-build:/root/go/src/yunion.io/x/sdnagent/_output \
		registry.cn-beijing.aliyuncs.com/yunionio/alpine-build:1.0-1 \
		/bin/sh -c "set -ex; cd /root/go/src/yunion.io/x/sdnagent; make; chown -R $$(id -u):$$(id -g) _output"
	docker build -t $(IMAGE_NAME_TAG) -f $(CURDIR)/build/docker/Dockerfile $(CURDIR)

docker-image-push:
	docker image push $(IMAGE_NAME_TAG)

.PHONY: all $(bins) rpm test
