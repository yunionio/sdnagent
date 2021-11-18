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
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-iptables/iptables"
	"github.com/digitalocean/go-openvswitch/ovs"
	"github.com/vishvananda/netlink"

	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"

	apis "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/mcclient/auth"
	mcclient_modules "yunion.io/x/onecloud/pkg/mcclient/modules/compute"
	"yunion.io/x/onecloud/pkg/util/iproute2"
	"yunion.io/x/onecloud/pkg/vpcagent/ovn/mac"

	"yunion.io/x/sdnagent/pkg/agent/utils"
)

const (
	pnamePrefix = "v-"
	pnameSuffix = "-p"
)

type ovnReq struct {
	ctx     context.Context
	guestId string
	nics    []*utils.GuestNIC
	r       chan utils.Empty
}

type ovnMan struct {
	hostId string
	ip     string // fetch from region
	mac    string // hash

	guestNics map[string][]*utils.GuestNIC
	watcher   *serversWatcher
	c         chan *ovnReq

	vpcPnames map[string]string
}

func newOvnMan(watcher *serversWatcher) *ovnMan {
	man := &ovnMan{
		watcher:   watcher,
		guestNics: map[string][]*utils.GuestNIC{},
		c:         make(chan *ovnReq),

		vpcPnames: map[string]string{},
	}
	return man
}

func (man *ovnMan) integrationBridge() string {
	return man.watcher.hostConfig.OvnIntegrationBridge
}

func (man *ovnMan) mappedBridge() string {
	return man.watcher.hostConfig.OvnMappedBridge
}

func (man *ovnMan) Start(ctx context.Context) {
	wg := ctx.Value("wg").(*sync.WaitGroup)
	defer wg.Done()

	refreshTicker := time.NewTicker(OvnManRefreshRate)
	defer refreshTicker.Stop()
	for {
		select {
		case req := <-man.c:
			man.guestNics[req.guestId] = req.nics
			man.ensureGuestFlows(req.ctx, req.guestId)
			req.r <- utils.Empty{}
		case <-refreshTicker.C:
			man.cleanup(ctx)
			man.refresh(ctx)
		case <-ctx.Done():
			log.Infof("ovn man bye")
			return
		}
	}
}

func (man *ovnMan) setIpMac(ctx context.Context) error {
	man.mac = mac.HashVpcHostDistgwMac(man.hostId)
	{
		hc := man.watcher.hostConfig
		apiVer := ""
		s := auth.GetAdminSession(ctx, hc.Region, apiVer)
		obj, err := mcclient_modules.Hosts.Get(s, man.hostId, nil)
		if err != nil {
			return errors.Wrapf(err, "GET host %s", man.hostId)
		}
		man.ip, _ = obj.GetString("ovn_mapped_ip_addr")
		if man.ip == "" {
			return errors.Errorf("Host %s has no mapped addr", man.hostId)
		}
	}

	if err := man.ensureMappedBridge(ctx); err != nil {
		return err
	}
	man.ensureBasicFlows(ctx)
	return nil
}

func (man *ovnMan) ensureMappedBridge(ctx context.Context) error {
	{
		args := []string{
			"ovs-vsctl",
			"--", "--may-exist", "add-br", man.mappedBridge(),
			"--", "set", "Bridge", man.mappedBridge(), fmt.Sprintf("other-config:hwaddr=%s", man.mac),
		}
		if err := utils.RunOvsctl(ctx, args); err != nil {
			return errors.Wrap(err, "ovn: ensure mapped bridge")
		}
	}

	if err := iproute2.NewLink(man.mappedBridge()).Up().Err(); err != nil {
		return errors.Wrapf(err, "ovn: set link %s up", man.mappedBridge())
	}

	if err := iproute2.NewAddress(man.mappedBridge(), fmt.Sprintf("%s/%d", man.ip, apis.VpcMappedIPMask)).Exact().Err(); err != nil {
		return errors.Wrapf(err, "ovn: set %s address %s", man.mappedBridge(), man.ip)
	}

	{
		ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
		if err != nil {
			return errors.Wrap(err, "ipt client")
		}
		var (
			p    = apis.VpcMappedCidr()
			tbl  = "nat"
			chn  = "POSTROUTING"
			spec = []string{
				"-s", p.String(),
				"-m", "comment", "--comment", "sdnagent: ovn distgw",
				"-j", "MASQUERADE",
			}
		)
		if err := ipt.AppendUnique(tbl, chn, spec...); err != nil {
			return errors.Wrapf(err, "ovn: append POSTROUTING masq rule")
		}
	}
	return nil
}

func (man *ovnMan) ensureGeneveFastpath(ctx context.Context) {
	dofunc := func(chain string) error {
		ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
		if err != nil {
			return errors.Wrap(err, "ipt client")
		}

		var (
			tbl     = "filter"
			chn     = chain
			comment = fmt.Sprintf("sdnagent: %s fastpath for geneve", chain)
			spec    = []string{
				"-p", "udp",
				"--dport", "6081",
				"-m", "comment", "--comment", comment,
				"-j", "ACCEPT",
			}
		)
		rules, err := ipt.List(tbl, chain)
		if err != nil {
			return errors.Wrapf(err, "list %q chain of %q table", chain, tbl)
		}
		if len(rules) > 1 {
			r0 := rules[0]
			if strings.Contains(r0, rules[1]) {
				return nil
			}
		}

		for first := true; ; {
			if err := ipt.Delete(tbl, chn, spec...); err != nil {
				break
			}
			if first {
				log.Warningf("try telling calico to use FELIX_CHAININSERTMODE=Append instead of Insert by default")
				first = false
			}
		}
		log.Infof("inserting %s", comment)
		if err := ipt.Insert(tbl, chn, 1, spec...); err != nil {
			return errors.Wrapf(err, "insert %q", strings.Join(spec, ","))
		}
		return nil
	}
	for _, c := range []string{"INPUT", "OUTPUT"} {
		if err := dofunc(c); err != nil {
			log.Errorf("ensureGeneveFastpath: %s: %v", c, err)
		}
	}
}

func (man *ovnMan) ensureBasicFlows(ctx context.Context) {
	var (
		p       = apis.VpcMappedCidr()
		hexMac  = "0x" + strings.TrimLeft(strings.ReplaceAll(man.mac, ":", ""), "0")
		actions = []string{
			"move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[]",
			fmt.Sprintf("load:%s->NXM_OF_ETH_SRC[]", hexMac),
			"load:0x2->NXM_OF_ARP_OP[]",
			fmt.Sprintf("load:%s->NXM_NX_ARP_SHA[]", hexMac),
			"move:NXM_OF_ARP_TPA[]->NXM_OF_ARP_SPA[]",
			"move:NXM_NX_ARP_SHA[]->NXM_NX_ARP_THA[]",
			"move:NXM_OF_ARP_SPA[]->NXM_OF_ARP_TPA[]",
			"in_port",
		}
	)
	flows := []*ovs.Flow{
		utils.F(0, 3050,
			fmt.Sprintf("in_port=LOCAL,arp,arp_op=1,arp_tpa=%s", p.String()),
			strings.Join(actions, ",")),
		utils.F(0, 32000,
			fmt.Sprintf("ip,nw_dst=%s", p.String()),
			"drop"),
	}
	flowman := man.watcher.agent.GetFlowMan(man.mappedBridge())
	flowman.updateFlows(ctx, "o", flows)
}

func (man *ovnMan) ensureMappedBridgeVpcPort(ctx context.Context, vpcId string) error {
	var (
		args       []string
		mine, peer = man.pnamePair(vpcId)
		ifaceId    = fmt.Sprintf("vpc-h/%s/%s", vpcId, man.hostId)
	)
	args = []string{
		"ovs-vsctl",
		"--", "--may-exist", "add-port", man.mappedBridge(), mine,
		"--", "set", "Interface", mine, "type=patch", fmt.Sprintf("options:peer=%s", peer),
		"--", "--may-exist", "add-port", man.integrationBridge(), peer,
		"--", "set", "Interface", peer, "type=patch", fmt.Sprintf("options:peer=%s", mine), fmt.Sprintf("external_ids:iface-id=%s", ifaceId),
	}
	if err := utils.RunOvsctl(ctx, args); err != nil {
		return errors.Wrapf(err, "ovn: ensure port: vpc %s", vpcId)
	}
	return nil
}

func (man *ovnMan) ensureMappedBridgeVpcPortFlows(ctx context.Context, vpcId string) error {
	mine, _ := man.pnamePair(vpcId)
	psMine, err := utils.DumpPort(man.mappedBridge(), mine)
	if err != nil {
		return err
	}
	pnoMine := psMine.PortID
	flowman := man.watcher.agent.GetFlowMan(man.mappedBridge())
	flowman.updateFlows(ctx, mine, []*ovs.Flow{
		utils.F(0, 30000, fmt.Sprintf("in_port=%d", pnoMine), "drop"),
	})
	return nil
}

func (man *ovnMan) pnamePair(vpcId string) (string, string) {
	var (
		base string
		ok   bool
	)
	if base, ok = man.vpcPnames[vpcId]; !ok {
		base = fmt.Sprintf("%s%s", pnamePrefix, vpcId[:15-len(pnamePrefix)-len(pnameSuffix)])
		man.vpcPnames[vpcId] = base
	}
	return base, base + pnameSuffix
}

func (man *ovnMan) ensureGuestFlows(ctx context.Context, guestId string) {
	var (
		nics   = man.guestNics[guestId]
		flows  []*ovs.Flow
		vpcIds = map[string]utils.Empty{}
	)
	for _, nic := range nics {
		vpcId := nic.Vpc.Id
		if vpcId == "" {
			continue
		}
		if _, ok := vpcIds[vpcId]; !ok {
			vpcIds[vpcId] = utils.Empty{}
			if err := man.ensureMappedBridgeVpcPort(ctx, vpcId); err != nil {
				log.Errorln(err)
				continue
			}
			if err := man.ensureMappedBridgeVpcPortFlows(ctx, vpcId); err != nil {
				log.Errorln(err)
				continue
			}
		}

		var (
			mine, _ = man.pnamePair(vpcId)
			pnoMine int
		)
		if psMine, err := utils.DumpPort(man.mappedBridge(), mine); err != nil {
			log.Errorf("ovn: dump port %s %s: %v", man.mappedBridge(), mine, err)
			continue
		} else {
			pnoMine = psMine.PortID
		}
		flows = append(flows,
			utils.F(0, 33000,
				fmt.Sprintf("in_port=LOCAL,ip,nw_dst=%s", nic.Vpc.MappedIpAddr),
				fmt.Sprintf("mod_dl_dst:%s,mod_nw_dst:%s,output:%d", apis.VpcMappedGatewayMac, nic.IP, pnoMine),
			),
			utils.F(0, 31000,
				fmt.Sprintf("in_port=%d,dl_src=%s,ip,nw_src=%s", pnoMine, apis.VpcMappedGatewayMac, nic.IP),
				fmt.Sprintf("mod_dl_dst:%s,mod_nw_src:%s,LOCAL", man.mac, nic.Vpc.MappedIpAddr),
			),
		)
	}
	flowman := man.watcher.agent.GetFlowMan(man.mappedBridge())
	flowman.updateFlows(ctx, guestId, flows)
}

func (man *ovnMan) SetHostId(ctx context.Context, hostId string) {
	if man.hostId == "" {
		man.hostId = hostId
		if err := man.setIpMac(ctx); err != nil { // TODO make it a readiness check
			log.Errorf("set ip mac: %s", err)
		}
		return
	}
	if man.hostId == hostId {
		return
	}
	// quit on host id change
}

func (man *ovnMan) SetGuestNICs(ctx context.Context, guestId string, nics []*utils.GuestNIC) {
	req := &ovnReq{
		ctx:     ctx,
		guestId: guestId,
		nics:    nics,
		r:       make(chan utils.Empty),
	}
	man.c <- req
	<-req.r
}

func (man *ovnMan) cleanup(ctx context.Context) {
	defer log.Infoln("ovn: clean done")

	// cleanup guestId without nics
	for guestId, nics := range man.guestNics {
		if len(nics) == 0 {
			delete(man.guestNics, guestId)
		}
	}

	// remove unused vpc patch ports
	vpcIds := map[string]utils.Empty{}
	for guestId, nics := range man.guestNics {
		guestId = guestId
		for _, nic := range nics {
			vpcIds[nic.Vpc.Id] = utils.Empty{}
		}
	}
	for vpcId := range man.vpcPnames {
		if _, ok := vpcIds[vpcId]; !ok {
			delete(man.vpcPnames, vpcId)
		}
	}

	listPorts := func(br string) (map[string]utils.Empty, bool) {
		cli := ovs.New().VSwitch
		if brs, err := cli.ListBridges(); err != nil {
			log.Errorf("list bridges: %v", err)
			return nil, false
		} else {
			found := false
			for _, got := range brs {
				if got == br {
					found = true
				}
			}
			if !found {
				return nil, true
			}
		}
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
	pnamesMine, ok := listPorts(man.mappedBridge())
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
			if !strings.HasPrefix(pname, pnamePrefix) {
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
	delPorts(man.mappedBridge(), pnamesMine)
	delPorts(man.integrationBridge(), pnamesPeer)
}

func (man *ovnMan) refresh(ctx context.Context) {
	defer log.Infoln("ovn: refresh done")
	man.ensureGeneveFastpath(ctx)
	if man.hostId != "" {
		if err := man.setIpMac(ctx); err != nil {
			log.Errorf("ovn: refresh: set ip mac: %v", err)
			return
		}
		for guestId := range man.guestNics {
			man.ensureGuestFlows(ctx, guestId)
		}
	}
}

// NOTE: KEEP THIS IN SYNC WITH CODE ABOVE
//
// 33000 in_port=LOCAL,nw_dst=VM_MAPPED,actions=mod_dl_dst:lr_mac,mod_nw_dst:VM_IP,output:brvpcp
// 32000 ip,nw_dst=100.64.0.0/17,actions=drop
// 31000 in_port=brvpcp,dl_src=lr_mac,ip,nw_src=VM_IP,actions=mod_dl_dst:man.mac,mod_nw_src:VM_MAPPED,LOCAL
// 30000 in_port=brvpcp,actions=drop
//
//  3050 in_port=LOCAL,arp,arp_op=1,...
