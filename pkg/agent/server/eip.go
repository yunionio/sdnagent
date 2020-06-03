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
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/digitalocean/go-openvswitch/ovs"
	"github.com/vishvananda/netlink"

	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"

	apis "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/util/iproute2"
	"yunion.io/x/onecloud/pkg/vpcagent/apihelper"
	agentmodels "yunion.io/x/onecloud/pkg/vpcagent/models"

	"yunion.io/x/sdnagent/pkg/agent/utils"
)

const (
	pnameEipPrefix = "ev-"
	pnameEipSuffix = "-p"
)

type eipMan struct {
	agent     *AgentServer
	vpcPnames map[string]string

	ip  string // fetch from region
	mac string // hash
}

func newEipMan(agent *AgentServer) *eipMan {
	man := &eipMan{
		agent:     agent,
		vpcPnames: map[string]string{},
	}
	return man
}

func (man *eipMan) integrationBridge() string {
	return man.agent.hostConfig.OvnIntegrationBridge
}

func (man *eipMan) eipBridge() string {
	return man.agent.hostConfig.OvnEipBridge
}

func (man *eipMan) Start(ctx context.Context) {
	wg := ctx.Value("wg").(*sync.WaitGroup)
	defer wg.Done()

	if err := man.setIpMac(ctx); err != nil {
		log.Errorf("eip: set ip mac: %v", err)
	}

	refreshTicker := time.NewTicker(EipManRefreshRate)
	defer refreshTicker.Stop()

	var apih *apihelper.APIHelper
	{
		modelSets := agentmodels.NewModelSets()
		apiOpts := &apihelper.Options{
			CommonOptions: man.agent.hostConfig.CommonOptions,
			SyncInterval:  5,
			ListBatchSize: 1024,
		}
		var err error
		apih, err = apihelper.NewAPIHelper(apiOpts, modelSets)
		if err != nil {
			panic("apihellper: %s" + err.Error())
		}
		wg.Add(1)
		go apih.Start(ctx)
	}

	var mss *agentmodels.ModelSets
	for {
		select {
		case imss := <-apih.ModelSets():
			log.Infof("eip: got new data from api helper")
			mss = imss.(*agentmodels.ModelSets)
			man.run(ctx, mss)
			man.cleanup(ctx, mss)
		case <-refreshTicker.C:
			if mss != nil {
				man.refresh(ctx, mss)
			}
		case <-ctx.Done():
			log.Infof("eip man bye")
			return
		}
	}
}

func (man *eipMan) setIpMac(ctx context.Context) error {
	man.mac = apis.VpcEipGatewayMac3
	man.ip = apis.VpcEipGatewayIP3().String()

	if err := man.ensureEipBridge(ctx); err != nil {
		return err
	}

	return nil
}

func (man *eipMan) ensureEipBridge(ctx context.Context) error {
	{
		args := []string{
			"ovs-vsctl",
			"--", "--may-exist", "add-br", man.eipBridge(),
			"--", "set", "Bridge", man.eipBridge(), fmt.Sprintf("other-config:hwaddr=%s", man.mac),
			"--", "set", "Interface", man.eipBridge(), fmt.Sprintf("mtu_request=%d", man.agent.hostConfig.GetOverlayMTU()),
		}
		if err := man.exec(ctx, args); err != nil {
			return errors.Wrap(err, "eip: ensure eip bridge")
		}
	}

	if err := iproute2.NewLink(man.eipBridge()).Up().Err(); err != nil {
		return errors.Wrapf(err, "eip: set link %s up", man.eipBridge())
	}

	if err := iproute2.NewAddress(man.eipBridge(), fmt.Sprintf("%s/%d", man.ip, apis.VpcEipGatewayIPMask)).Exact().Err(); err != nil {
		return errors.Wrapf(err, "eip: set %s address %s", man.eipBridge(), man.ip)
	}
	return nil
}

func (man *eipMan) ensureEipBridgeVpcPort(ctx context.Context, vpcId string) error {
	var (
		args       []string
		mine, peer = man.pnamePair(vpcId)
		ifaceId    = fmt.Sprintf("vpc-ep/%s/%s", vpcId, apis.VpcEipGatewayIP3())
	)
	args = []string{
		"ovs-vsctl",
		"--", "--may-exist", "add-port", man.eipBridge(), mine,
		"--", "set", "Interface", mine, "type=patch", fmt.Sprintf("options:peer=%s", peer),
		"--", "--may-exist", "add-port", man.integrationBridge(), peer,
		"--", "set", "Interface", peer, "type=patch", fmt.Sprintf("options:peer=%s", mine), fmt.Sprintf("external_ids:iface-id=%s", ifaceId),
	}
	if err := man.exec(ctx, args); err != nil {
		return errors.Wrapf(err, "eip: ensure port: vpc %s", vpcId)
	}
	return nil
}

func (man *eipMan) exec(ctx context.Context, args []string) error {
	if len(args) == 0 {
		panic("exec: empty args")
	}
	tos := func(args []string) string {
		s := ""
		for _, arg := range args {
			if arg != "--" {
				s += " " + arg
			} else {
				s += " \\\n  " + arg
			}
		}
		return s
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	_, err := cmd.Output()
	if err != nil {
		s := tos(args)
		err = errors.Wrap(err, s)
		return err
	}
	return nil
}

func (man *eipMan) pnamePair(vpcId string) (string, string) {
	var (
		base string
		ok   bool
	)
	if base, ok = man.vpcPnames[vpcId]; !ok {
		base = fmt.Sprintf("%s%s", pnameEipPrefix, vpcId[:15-len(pnameEipPrefix)-len(pnameEipSuffix)])
		man.vpcPnames[vpcId] = base
	}
	return base, base + pnameEipSuffix
}

func (man *eipMan) run(ctx context.Context, mss *agentmodels.ModelSets) {
	var (
		flows = []*ovs.Flow{
			utils.F(0, 1000, "", "drop"),
		}
		vpcIds = map[string]utils.Empty{}
		route  = iproute2.NewRoute(man.eipBridge())
	)
	for _, gn := range mss.Guestnetworks {
		eip := gn.Elasticip
		if eip == nil {
			continue
		}
		var (
			network = gn.Network
			vpc     = network.Vpc
			vpcId   = vpc.Id
		)

		if _, ok := vpcIds[vpcId]; !ok {
			vpcIds[vpcId] = utils.Empty{}
			if err := man.ensureEipBridgeVpcPort(ctx, vpcId); err != nil {
				log.Errorln(err)
				continue
			}
		}

		var (
			mine, _ = man.pnamePair(vpcId)
			pnoMine int
		)
		if psMine, err := utils.DumpPort(man.eipBridge(), mine); err != nil {
			log.Errorf("eip: dump port %s %s: %v", man.eipBridge(), mine, err)
			continue
		} else {
			pnoMine = psMine.PortID
		}

		var (
			vpcIp      = gn.IpAddr
			eipIp      = eip.IpAddr
			hexMac     = "0x" + strings.TrimLeft(strings.ReplaceAll(apis.VpcEipGatewayMac, ":", ""), "0")
			hexMac3    = "0x" + strings.TrimLeft(strings.ReplaceAll(man.mac, ":", ""), "0")
			arpactions = []string{
				"move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[]",
				fmt.Sprintf("load:%s->NXM_OF_ETH_SRC[]", hexMac3),
				"load:0x2->NXM_OF_ARP_OP[]",
				fmt.Sprintf("load:%s->NXM_NX_ARP_SHA[]", hexMac),
				"move:NXM_OF_ARP_TPA[]->NXM_OF_ARP_SPA[]",
				"move:NXM_NX_ARP_SHA[]->NXM_NX_ARP_THA[]",
				"move:NXM_OF_ARP_SPA[]->NXM_OF_ARP_TPA[]",
				"in_port",
			}
		)
		flows = append(flows,
			utils.F(0, 33000,
				fmt.Sprintf("in_port=%d,dl_src=%s,ip,nw_src=%s", pnoMine, apis.VpcEipGatewayMac, vpcIp),
				fmt.Sprintf("mod_dl_dst:%s,mod_nw_src:%s,LOCAL", man.mac, eipIp),
			),
			utils.F(0, 32000,
				fmt.Sprintf("in_port=LOCAL,ip,nw_dst=%s", eipIp),
				fmt.Sprintf("mod_dl_dst:%s,mod_nw_dst:%s,output:%d", apis.VpcEipGatewayMac, vpcIp, pnoMine),
			),
			utils.F(0, 31000,
				fmt.Sprintf("in_port=LOCAL,arp,arp_op=1,arp_tpa=%s", eipIp),
				strings.Join(arpactions, ","),
			),
		)
		route.Add(eipIp, "255.255.255.255", "")
	}
	if err := route.Err(); err != nil {
		log.Errorf("eip: route error: %v", err)
	}
	flowman := man.agent.GetFlowMan(man.eipBridge())
	flowman.updateFlows(ctx, "eipman", flows)
}

func (man *eipMan) cleanup(ctx context.Context, mss *agentmodels.ModelSets) {
	defer log.Infoln("eip: clean done")

	var (
		vpcIds    = map[string]utils.Empty{}
		routeDsts = map[string]utils.Empty{}
	)

	for _, gn := range mss.Guestnetworks {
		eip := gn.Elasticip
		if eip == nil {
			continue
		}
		var (
			network = gn.Network
			vpc     = network.Vpc
			vpcId   = vpc.Id
		)
		vpcIds[vpcId] = utils.Empty{}
		routeDsts[eip.IpAddr+"/32"] = utils.Empty{}
	}
	{
		cidr := apis.VpcEipGatewayCidr()
		routeDsts[cidr.String()] = utils.Empty{}
	}

	{ // remove unused routes
		h := iproute2.NewRoute(man.eipBridge())
		routes, err := h.List4()
		if err != nil {
			log.Errorf("list %s routes", man.eipBridge())
		} else {
			for _, r := range routes {
				var (
					dst    = r.Dst
					dstStr = dst.String()
				)
				if _, ok := routeDsts[dstStr]; !ok {
					h.DelByIPNet(dst)
				}
			}
		}
		if err := h.Err(); err != nil {
			log.Errorf("eip: clean routes: %v", err)
		}

	}

	{ // remove unused vpc patch ports
		for vpcId := range man.vpcPnames {
			if _, ok := vpcIds[vpcId]; !ok {
				delete(man.vpcPnames, vpcId)
			}
		}

		listPorts := func(br string) (map[string]utils.Empty, bool) {
			cli := ovs.New().VSwitch
			ports, err := cli.ListPorts(br)
			if err != nil {
				log.Errorf("list bridge ports: %s: %v", br, err)
				return nil, false
			}
			r := map[string]utils.Empty{}
			for _, pname := range ports {
				r[pname] = utils.Empty{}
			}
			return r, true
		}
		pnamesPeer, ok := listPorts(man.integrationBridge())
		if !ok {
			return
		}
		pnamesMine, ok := listPorts(man.eipBridge())
		if !ok {
			return
		}
		for vpcId := range man.vpcPnames {
			mine, peer := man.pnamePair(vpcId)
			delete(pnamesMine, mine)
			delete(pnamesPeer, peer)
		}
		delPorts := func(br string, pnames map[string]utils.Empty) {
			cli := ovs.New().VSwitch
			for pname := range pnames {
				if !strings.HasPrefix(pname, pnameEipPrefix) {
					continue
				}
				if _, err := netlink.LinkByName(pname); err == nil {
					continue
				}
				log.Infof("del bridge port: %s %s", br, pname)
				if err := cli.DeletePort(br, pname); err != nil {
					log.Errorf("del bridge port: %s %s: %v", br, pname, err)
				}
			}
		}
		delPorts(man.eipBridge(), pnamesMine)
		delPorts(man.integrationBridge(), pnamesPeer)
	}
}

func (man *eipMan) refresh(ctx context.Context, mss *agentmodels.ModelSets) {
	defer log.Infoln("eip: refresh done")

	if err := man.setIpMac(ctx); err != nil {
		log.Errorf("eip: refresh: set ip mac: %v", err)
		return
	}
	man.run(ctx, mss)
}

// NOTE: KEEP THIS IN SYNC WITH CODE ABOVE
//
// 33000 in_port=brvpcp,dl_src=ee:ee:ee:ee:ee:ef,ip,nw_src=VM_IP,actions=mod_dl_dst:man.mac,mod_nw_src:VM_EIP,output=LOCAL
// 32000 in_port=LOCAL,ip,nw_dst=VM_EIP/32,actions=mod_dl_dst:ee:ee:ee:ee:ee:ef,mod_nw_dst:VM_IP,output=brvpcp
// 31000 in_port=LOCAL,arp,arp_op=1,arp_tpa=VM_EIP/32,actions=move:NXM_OF_ETH_SRC->NXM_OF_ETH_DST,load:man.mac->NXM_OF_ETH_SRC,load:0x2->NXM_OF_ARP_OP,load:0xeeeeeeeeeeef->NXM_NX_ARP_SHA,move:NXM_OF_ARP_TPA->NXM_OF_ARP_SPA,move:NXM_NX_ARP_SHA->NXM_NX_ARP_THA,move:NXM_OF_ARP_SPA->NXM_OF_ARP_TPA,output=in_port"
//
//  1000 actions=drop
