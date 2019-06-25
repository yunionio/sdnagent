BINDIR:=_output/bin

GO_BUILD_FLAGS:=-mod vendor
GO_BUILD:=go build $(GO_BUILD_FLAGS)
GO_TEST:=go test $(GO_BUILD_FLAGS)
export GO111MODULE:=on

all: bins pkg/agent/proto/agent.pb.go

bins:
	mkdir -p $(BINDIR)
	$(GO_BUILD) yunion.io/x/sdnagent/pkg/agent
	$(GO_BUILD) -o $(BINDIR)/sdnagent yunion.io/x/sdnagent/cmd/sdnagent
	$(GO_BUILD) -o $(BINDIR)/sdncli yunion.io/x/sdnagent/cmd/sdncli

pkg/agent/proto/agent.pb.go: pkg/agent/proto/agent.proto
	protoc -I pkg/agent/proto pkg/agent/proto/agent.proto --go_out=plugins=grpc:pkg/agent/proto

pkg/agent/proto/agent_pb2.py: pkg/agent/proto/agent.proto
	python -m grpc_tools.protoc -Ipkg/agent/proto --python_out=pkg/agent/proto --grpc_python_out=pkg/agent/proto pkg/agent/proto/agent.proto

test:
	$(GO_TEST)  -v ./...

rpm: bins
	EXTRA_BINS=sdncli \
		 $(CURDIR)/build/build.sh sdnagent

.PHONY: all bins rpm test
