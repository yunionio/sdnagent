package agent

import (
	"fmt"

	"google.golang.org/grpc"

	"yunion.io/x/sdnagent/pkg/agent/common"
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

func NewClient() (*AgentClient, error) {
	conn, err := grpc.Dial("unix://"+common.UnixSocketFile, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	c := &AgentClient{
		VSwitch:  pb.NewVSwitchClient(conn),
		Openflow: pb.NewOpenflowClient(conn),
	}
	return c, nil
}
