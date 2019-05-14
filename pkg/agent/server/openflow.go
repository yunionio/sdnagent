// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
