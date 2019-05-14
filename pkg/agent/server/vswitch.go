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
