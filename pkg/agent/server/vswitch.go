package server

import (
	"context"

	pb "yunion.io/x/sdnagent/pkg/agent/proto"

	"github.com/digitalocean/go-openvswitch/ovs"
)

type vSwitchService struct {
	agent      *AgentServer
	vSwitchCli *ovs.VSwitchService
}

func newVSwitchService(agent *AgentServer) *vSwitchService {
	return &vSwitchService{
		agent:      agent,
		vSwitchCli: ovs.New().VSwitch,
	}
}

func (s *vSwitchService) newResponse(err error) *pb.Response {
	if err == nil {
		return &pb.Response{Code: 0, Mesg: "ok"}
	}
	return &pb.Response{Code: 1, Mesg: err.Error()}
}

func (s *vSwitchService) AddBridge(ctx context.Context, in *pb.AddBridgeRequest) (*pb.Response, error) {
	err := s.vSwitchCli.AddBridge(in.Bridge)
	return s.newResponse(err), nil
}
func (s *vSwitchService) DelBridge(ctx context.Context, in *pb.DelBridgeRequest) (*pb.Response, error) {
	err := s.vSwitchCli.DeleteBridge(in.Bridge)
	return s.newResponse(err), nil
}
func (s *vSwitchService) AddBridgePort(ctx context.Context, in *pb.AddBridgePortRequest) (*pb.Response, error) {
	err := s.vSwitchCli.AddPort(in.Bridge, in.Port)
	return s.newResponse(err), nil
}
func (s *vSwitchService) DelBridgePort(ctx context.Context, in *pb.DelBridgePortRequest) (*pb.Response, error) {
	err := s.vSwitchCli.DeletePort(in.Bridge, in.Port)
	return s.newResponse(err), nil
}
