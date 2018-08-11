package server

import (
	"context"
	"fmt"

	"github.com/digitalocean/go-openvswitch/ovs"
	pb "yunion.io/x/sdnagent/pkg/agent/proto"
)

type openflowService struct {
	agent *AgentServer
	ofCli *ovs.OpenFlowService
}

func newOpenflowService(agent *AgentServer) *openflowService {
	return &openflowService{
		agent: agent,
		ofCli: ovs.New().OpenFlow,
	}
}

func (s *openflowService) newResponse(err error) *pb.Response {
	if err == nil {
		return &pb.Response{Code: 0, Mesg: "ok"}
	}
	return &pb.Response{Code: 1, Mesg: err.Error()}
}

func (s *openflowService) AddFlow(ctx context.Context, in *pb.AddFlowRequest) (*pb.Response, error) {
	f, err := in.Flow.OvsFlow()
	if err != nil {
		err = fmt.Errorf("conversion to ovs.Flow error: %s", err)
		return s.newResponse(err), nil
	}
	flowman := s.agent.GetFlowMan(in.Bridge)
	flowman.AddFlow(ctx, f)
	return s.newResponse(nil), nil
}
func (s *openflowService) DelFlow(ctx context.Context, in *pb.DelFlowRequest) (*pb.Response, error) {
	f, err := in.Flow.OvsFlow()
	if err != nil {
		err = fmt.Errorf("conversion to ovs.Flow error: %s", err)
		return s.newResponse(err), nil
	}
	flowman := s.agent.GetFlowMan(in.Bridge)
	flowman.DelFlow(ctx, f)
	return s.newResponse(nil), nil
}

func (s *openflowService) SyncFlows(ctx context.Context, in *pb.SyncFlowsRequest) (*pb.Response, error) {
	flowman := s.agent.GetFlowMan(in.Bridge)
	flowman.SyncFlows(ctx)
	return s.newResponse(nil), nil
}

func (s *openflowService) DumpBridgePort(ctx context.Context, in *pb.DumpBridgePortRequest) (*pb.DumpBridgePortResponse, error) {
	ofPortStats, err := s.ofCli.DumpPort(in.Bridge, in.Port)
	if err != nil {
		resp := &pb.DumpBridgePortResponse{
			Code: 1,
			Mesg: err.Error(),
		}
		return resp, nil
	}
	resp := &pb.DumpBridgePortResponse{
		Code: 0,
		Mesg: "ok",
		PortStats: &pb.PortStats{
			PortNo: uint32(ofPortStats.PortID),
		},
	}
	return resp, nil
}
