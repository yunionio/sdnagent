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

package agent

import (
	"fmt"

	"google.golang.org/grpc"

	pb "yunion.io/x/sdnagent/pkg/agent/proto"
)

type AgentClient struct {
	VSwitch  pb.VSwitchClient
	Openflow pb.OpenflowClient
}

func (c *AgentClient) W(resp pb.CommonResponse, err error) error {
	if err != nil {
		return err
	}
	if resp.GetCode() != 0 {
		return fmt.Errorf("err %d: %s", resp.GetCode(), resp.GetMesg())
	}
	return nil
}

func NewClient(sockPath string) (*AgentClient, error) {
	conn, err := grpc.Dial("unix://"+sockPath, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	c := &AgentClient{
		VSwitch:  pb.NewVSwitchClient(conn),
		Openflow: pb.NewOpenflowClient(conn),
	}
	return c, nil
}
