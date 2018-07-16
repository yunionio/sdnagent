all: pkg/agent/proto/agent.pb.go
	mkdir -p _output/bin
	go build yunion.io/yunion-sdnagent/pkg/agent
	go build -o _output/bin/yunion-sdnagent yunion.io/yunion-sdnagent/cmd/sdnagent
	go build -o _output/bin/sdncli yunion.io/yunion-sdnagent/cmd/sdncli

pkg/agent/proto/agent.pb.go: pkg/agent/proto/agent.proto
	protoc -I pkg/agent/proto pkg/agent/proto/agent.proto --go_out=plugins=grpc:pkg/agent/proto

pkg/agent/proto/agent_pb2.py: pkg/agent/proto/agent.proto
	python -m grpc_tools.protoc -Ipkg/agent/proto --python_out=pkg/agent/proto --grpc_python_out=pkg/agent/proto pkg/agent/proto/agent.proto
