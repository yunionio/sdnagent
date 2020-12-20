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

package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"yunion.io/x/log"

	"yunion.io/x/sdnagent/pkg/agent"
	pb "yunion.io/x/sdnagent/pkg/agent/proto"
)

func flagSetMustGet(v interface{}, err error) interface{} {
	return v
}

func flagSetToFlowArgs(fs *pflag.FlagSet) *pb.Flow {
	must := flagSetMustGet
	flow := &pb.Flow{
		Cookie:   must(fs.GetUint64("cookie")).(uint64),
		Priority: must(fs.GetUint32("priority")).(uint32),
		Table:    must(fs.GetUint32("table")).(uint32),
		Matches:  must(fs.GetString("matches")).(string),
		Actions:  must(fs.GetString("actions")).(string),
	}
	return flow
}

func InitCmdFlags(cmd *cobra.Command) {
	switch cmd.Name() {
	case "addFlow", "delFlow":
		cmd.Flags().StringP("bridge", "b", "br0", "bridge")
		cmd.Flags().Uint64P("cookie", "c", 0, "flow cookie")
		cmd.Flags().Uint32P("priority", "p", 0, "flow priority")
		cmd.Flags().Uint32P("table", "t", 0, "flow table number")
		cmd.Flags().StringP("matches", "m", "", "flow match conditions")
		cmd.Flags().StringP("actions", "a", "normal", "flow actions")
	case "syncFlows":
		cmd.Flags().StringP("bridge", "b", "br0", "bridge")
	case "dumpBridgePort":
		cmd.Flags().StringP("bridge", "b", "br0", "bridge")
		cmd.Flags().StringP("port", "p", "", "port")
	}
}

func handleResponse(resp pb.CommonResponse, err error, fmt string) bool {
	if err != nil {
		log.Errorf("rpc failure: %s", err)
		return false
	}
	if resp.GetCode() != 0 {
		// todo make tempalte
		log.Errorf(fmt, resp.GetMesg())
		return false
	}
	return true
}

func DoCmd(cmd *cobra.Command) {
	sockPath, err := cmd.Flags().GetString("sock")
	if err != nil {
		log.Fatalf("get sock option: %v", err)
	}
	c, err := agent.NewClient(sockPath)
	if err != nil {
		log.Fatalf("client failure: %s", err)
	}
	bridge := flagSetMustGet(cmd.Flags().GetString("bridge")).(string)
	switch cmd.Name() {
	case "addFlow":
		flow := flagSetToFlowArgs(cmd.Flags())
		req := &pb.AddFlowRequest{
			Bridge: bridge,
			Flow:   flow,
		}
		resp, err := c.Openflow.AddFlow(context.Background(), req)
		handleResponse(resp, err, "addFlow failure: %s")
	case "delFlow":
		flow := flagSetToFlowArgs(cmd.Flags())
		req := &pb.DelFlowRequest{
			Bridge: bridge,
			Flow:   flow,
		}
		resp, err := c.Openflow.DelFlow(context.Background(), req)
		handleResponse(resp, err, "delFlow failure: %s")
	case "syncFlows":
		req := &pb.SyncFlowsRequest{
			Bridge: bridge,
		}
		resp, err := c.Openflow.SyncFlows(context.Background(), req)
		handleResponse(resp, err, "syncFlows failure: %s")
	case "dumpBridgePort":
		port := flagSetMustGet(cmd.Flags().GetString("port")).(string)
		req := &pb.DumpBridgePortRequest{
			Bridge: bridge,
			Port:   port,
		}
		resp, err := c.Openflow.DumpBridgePort(context.Background(), req)
		ok := handleResponse(resp, err, "dumpBridgePort failure: %s")
		if ok {
			fmt.Printf("%d\n", resp.PortStats.PortNo)
		}
	}
}
