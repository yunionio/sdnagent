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

package utils

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/digitalocean/go-openvswitch/ovs"
	"github.com/vishvananda/netlink"

	"yunion.io/x/log"
)

type FlowSource interface {
	Who() string
	FlowsMap() (map[string][]*ovs.Flow, error)
}

func F(table, priority int, matches, actions string) *ovs.Flow {
	txt := fmt.Sprintf("table=%d,priority=%d,%s,actions=%s", table, priority, matches, actions)
	of := &ovs.Flow{}
	err := of.UnmarshalText([]byte(txt))
	if err != nil {
		panic("bad flow: " + txt + ": " + err.Error())
	}
	return of
}

func t(m map[string]interface{}) func(string) string {
	return func(text string) string {
		t := template.Must(template.New("").Parse(text))
		b := &bytes.Buffer{}
		t.Execute(b, m)
		return b.String()
	}
}

func (h *HostLocal) t(text string) string {
	t := template.Must(template.New("").Parse(text))
	b := &bytes.Buffer{}
	t.Execute(b, h)
	return b.String()
}

func (h *HostLocal) Who() string {
	return "hostlocal." + h.Bridge
}

func (h *HostLocal) FlowsMap() (map[string][]*ovs.Flow, error) {
	ps, err := ovs.New().OpenFlow.DumpPort(h.Bridge, h.Ifname)
	if err != nil {
		return nil, err
	}
	m := map[string]interface{}{
		"MetadataPort": h.HostConfig.MetadataPort(),
		"K8SCidr":      h.HostConfig.K8sClusterCidr,
		"IP":           h.IP,
		"MAC":          h.MAC,
		"PortNoPhy":    ps.PortID,
	}
	T := t(m)
	flows := []*ovs.Flow{
		F(0, 40000, "ipv6", "drop"),
	}
	if h.HostConfig.K8sClusterCidr != nil {
		flows = append(flows, F(0, 30050, T("ip,nw_dst={{.K8SCidr}}"), T("mod_dl_dst:{{.MAC}},LOCAL")))
	}
	flows = append(flows,
		F(0, 29310, "in_port=LOCAL,tcp,nw_dst=169.254.169.254,tp_dst=80", T("normal")),
		F(0, 27200, "in_port=LOCAL", "normal"),
		F(0, 26900, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}}"), "normal"),
	)
	// NOTE we do not do check of existence of a "switch" guest and
	// silently "AllowSwitchVMs" here.  That could be deemed as unexpected
	// compromise for other guests.  Intentions must be explicit
	if h.HostConfig.AllowSwitchVMs {
		flows = append(flows,
			F(0, 23700, T("in_port={{.PortNoPhy}}"), "normal"),
		)
	} else {
		flows = append(flows,
			F(0, 23600, T("in_port={{.PortNoPhy}},dl_dst=01:00:00:00:00:00/01:00:00:00:00:00"), "normal"),
			F(0, 23500, T("in_port={{.PortNoPhy}}"), "drop"),
		)
	}
	return map[string][]*ovs.Flow{h.Bridge: flows}, nil
}

func (g *Guest) Who() string {
	return g.Id
}

func (g *Guest) getMetadataInfo(nic *GuestNIC) (mdIP string, mdMAC string, mdPortNo int, useLOCAL bool, err error) {
	useLOCAL = true
	route, err := RouteLookup(nic.IP)
	if err != nil {
		return
	}
	if route.Dev == nic.Bridge {
		return
	}

	link, err := netlink.LinkByName(route.Dev)
	if err != nil {
		return
	}
	vethLink, ok := link.(*netlink.Veth)
	if !ok {
		return
	}
	idx, err := netlink.VethPeerIndex(vethLink)
	if err != nil {
		return
	}
	linkPeer, err := netlink.LinkByIndex(idx)
	if err != nil {
		return
	}

	ofCli := ovs.New().OpenFlow
	{
		var (
			p     *ovs.PortStats
			addrs []netlink.Addr
		)
		{
			attrs := linkPeer.Attrs()
			p, err = ofCli.DumpPort(nic.Bridge, attrs.Name)
			if err != nil {
				var masterLink netlink.Link
				masterLink, err = netlink.LinkByIndex(attrs.MasterIndex)
				if err != nil {
					return
				}
				p, err = ofCli.DumpPort(nic.Bridge, masterLink.Attrs().Name)
				if err != nil {
					return
				}
			}
		}
		addrs, err = netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			return
		}
		if len(addrs) == 0 {
			return
		}
		err = nil
		useLOCAL = false
		mdIP = addrs[0].IP.String()
		mdMAC = link.Attrs().HardwareAddr.String()
		mdPortNo = p.PortID
	}
	return
}

func (g *Guest) FlowsMap() (map[string][]*ovs.Flow, error) {
	r := map[string][]*ovs.Flow{}
	allGood := true
	for _, nic := range g.NICs {
		if nic.PortNo <= 0 {
			allGood = false
			continue
		}
		ofCli := ovs.New().OpenFlow
		hcn := g.HostConfig.HostNetworkConfig(nic.Bridge)
		if hcn == nil {
			log.Warningf("guest %s port %s: no host network config for %s",
				g.Id, nic.IfnameHost, nic.Bridge)
			allGood = false
			continue
		}
		ps, err := ofCli.DumpPort(nic.Bridge, hcn.Ifname)
		if err != nil {
			log.Warningf("fetch phy port_no of %s,%s failed: %s",
				nic.Bridge, hcn.Ifname, err)
			allGood = false
			continue
		}
		portNoPhy := ps.PortID
		m := nic.Map()
		m["DHCPServerPort"] = g.HostConfig.DHCPServerPort
		m["PortNoPhy"] = portNoPhy
		{
			var mdPortAction string
			var mdInPort string
			mdIP, mdMAC, mdPortNo, useLOCAL, err := g.getMetadataInfo(nic)
			if useLOCAL {
				if err != nil {
					log.Warningf("find metadata: %v", err)
				}
				{
					ip, mac, err := hcn.IPMAC()
					if err != nil {
						log.Warningf("host network find ip mac: %v", err)
						continue
					}
					mdIP = ip.String()
					mdMAC = mac.String()
				}
				mdPortNo = portNoPhy
				mdInPort = "LOCAL"
				mdPortAction = "LOCAL"
			} else {
				mdInPort = fmt.Sprintf("%d", mdPortNo)
				mdPortAction = fmt.Sprintf("output:%d", mdPortNo)
			}
			m["MetadataServerPort"] = g.HostConfig.MetadataPort()
			m["MetadataServerIP"] = mdIP
			m["MetadataServerMAC"] = mdMAC
			m["MetadataPortInPort"] = mdInPort
			m["MetadataPortAction"] = mdPortAction
		}
		T := t(m)
		if nic.VLAN > 1 {
			m["_dl_vlan"] = T("dl_vlan={{.VLAN}}")
		} else {
			m["_dl_vlan"] = T("vlan_tci={{.VLANTci}}")
		}
		flows := []*ovs.Flow{}
		if g.HostConfig.K8sClusterCidr != nil {
			m["K8SCidr"] = g.HostConfig.K8sClusterCidr
			flows = append(flows,
				F(0, 30040, T("in_port=LOCAL,ip,nw_src={{.K8SCidr}},nw_dst={{.IP}}"),
					T("mod_dl_dst:{{.MAC}},output:{{.PortNo}}")),
			)
		}
		flows = append(flows,
			F(0, 29200,
				T("in_port={{.MetadataPortInPort}},tcp,nw_dst={{.IP}},tp_src={{.MetadataServerPort}}"),
				T("mod_dl_dst:{{.MAC}},mod_nw_src:169.254.169.254,mod_tp_src:80,output:{{.PortNo}}")),
			F(0, 29300, T("in_port={{.PortNo}},tcp,nw_dst=169.254.169.254,tp_dst=80"),
				T("mod_dl_dst:{{.MetadataServerMAC}},mod_nw_dst:{{.MetadataServerIP}},mod_tp_dst:{{.MetadataServerPort}},{{.MetadataPortAction}}")),
			F(0, 28400, T("in_port={{.PortNo}},udp,tp_src=68,tp_dst=67"), T("mod_tp_dst:{{.DHCPServerPort}},local")),
			F(0, 28300, T("in_port=LOCAL,dl_dst={{.MAC}},udp,tp_src={{.DHCPServerPort}},tp_dst=68"), T("mod_tp_src:67,output:{{.PortNo}}")),
			F(0, 26700, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}}"), "normal"),
		)
		if !g.SrcMacCheck() {
			flows = append(flows, F(0, 24670, T("in_port={{.PortNo}}"), "normal"))
		} else {
			flows = append(flows,
				F(0, 25770, T("in_port={{.PortNo}},arp,dl_src={{.MAC}},arp_sha={{.MAC}},arp_spa={{.IP}}"), "normal"),
				F(0, 25760, T("in_port={{.PortNo}},arp"), "drop"),
				F(0, 24660, T("in_port={{.PortNo}}"), "drop"),
			)
		}
		flows = append(flows, g.SecurityRules.Flows(g, m)...)
		if fs, ok := r[nic.Bridge]; ok {
			flows = append(fs, flows...)
		}
		r[nic.Bridge] = flows
	}
	if !allGood {
		return r, fmt.Errorf("not all nics ready")
	}
	return r, nil
}

func (sr *SecurityRules) Flows(g *Guest, data map[string]interface{}) []*ovs.Flow {
	T := t(data)
	data["_in_port_vm"] = "reg0=0x10000/0x10000"
	data["_in_port_not_vm"] = "reg0=0x0/0x10000"
	loadReg0BitVm := "load:0x1->NXM_NX_REG0[16]" // "0x1->" is important, not "1->"
	var loadZone, loadZoneDstVM string
	{
		s := fmt.Sprintf("0x%x", data["CT_ZONE"])
		if data["CT_ZONE"] == 0 {
			s = "0" // always 0, not 0x0
		}
		loadZone = fmt.Sprintf("load:%s->NXM_NX_REG0[0..15]", s)
		loadZoneDstVM = fmt.Sprintf("load:%s->NXM_NX_REG1[0..15]", s)
	}

	flows := []*ovs.Flow{}
	// table 0
	// table 1 sec_CT
	flows = append(flows,
		F(0, 27300, T("in_port=LOCAL,dl_dst={{.MAC}},ip"),
			loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
	)
	if !g.SrcIpCheck() {
		flows = append(flows,
			F(0, 26870, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ip"),
				loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
			F(0, 25870, T("in_port={{.PortNo}},dl_src={{.MAC}},ip"),
				loadReg0BitVm+","+loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
			F(0, 24770, T("dl_dst={{.MAC}},ip"),
				loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
		)
	} else {
		flows = append(flows,
			F(0, 26870, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ip,nw_dst={{.IP}}"),
				loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
			F(0, 26860, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ip"), "drop"),
			F(0, 25870, T("in_port={{.PortNo}},dl_src={{.MAC}},ip,nw_src={{.IP}}"),
				loadReg0BitVm+","+loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
			F(0, 25860, T("in_port={{.PortNo}},dl_src={{.MAC}},ip"), "drop"),
			F(0, 24770, T("dl_dst={{.MAC}},ip,nw_dst={{.IP}}"),
				loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
			F(0, 24760, T("dl_dst={{.MAC}},ip"), "drop"),
		)
	}
	flows = append(flows,
		F(0, 25600, T("in_port={{.PortNo}},dl_src={{.MAC}}"), "normal"),

		// ct_state= flags order matters
		F(1, 7900, "ip,ct_state=+inv+trk", "drop"),
		F(1, 7800, T("ip,ct_state=+new+trk,{{._in_port_not_vm}}"), "resubmit(,3)"),
		F(1, 7700, T("ip,ct_state=+new+trk,{{._in_port_vm}}"), "resubmit(,2)"),
		F(1, 7600, "ip", "resubmit(,4)"),
		F(4, 5600, T("ip,dl_dst={{.MAC}}"), loadZoneDstVM+",resubmit(,5),"),
		F(4, 5500, "ip", "ct(commit,zone=NXM_NX_REG0[0..15]),normal"),
	)

	// table sec_CT_OUT
	prioOut := 40000
	matchOut := T("in_port={{.PortNo}}")
	for _, r := range sr.outRules {
		if prioOut <= 20 {
			log.Errorf("%s: %q generated too many out rules",
				data["IP"], sr.OutRulesString())
			break
		}
		action := "drop"
		if r.OvsActionAllow() {
			action = "resubmit(,3)"
		}
		for _, m := range r.OvsMatches() {
			flows = append(flows, F(2, prioOut, matchOut+","+m, action))
			prioOut -= 1
		}
	}

	// table sec_CT_IN
	prioIn := 40000
	matchIn := T("dl_dst={{.MAC}}")
	actionAllowIn := loadZoneDstVM + ",resubmit(,5)"
	for _, r := range sr.inRules {
		if prioIn <= 30 {
			log.Errorf("%s: %q generated too many in rules",
				data["IP"], sr.InRulesString())
			break
		}
		action := "drop"
		if r.OvsActionAllow() {
			action = actionAllowIn
		}
		for _, m := range r.OvsMatches() {
			flows = append(flows, F(3, prioIn, matchIn+","+m, action))
			prioIn -= 1
		}
	}
	// NOTE Traffics enter sec_XX table by dl_dst=MAC_VM, except the egress
	// rule in_port=PORT_VM.  The following rule are for VM accessing hosts
	// other than locally managed VMs
	flows = append(flows, F(3, 30, "ip", "ct(commit,zone=NXM_NX_REG0[0..15]),normal"))
	flows = append(flows,
		F(5, 20, T("ip,{{._in_port_not_vm}}"), "ct(commit,zone=NXM_NX_REG1[0..15]),normal"),
		F(5, 10, T("ip,{{._in_port_vm}}"), "ct(commit,zone=NXM_NX_REG1[0..15]),ct(commit,zone=NXM_NX_REG0[0..15]),normal"),
	)
	return flows
}

// NOTE: KEEP THIS IN SYNC WITH CODE ABOVE
//
//	grep -oE '\<F\([0-9].*' pkg/agent/utils/flowsource.go  | sort -k 1.3,1.4n -k2r
//
// Assumptions
//
//  - MAC is unique, this can be an issue when allow_switch_vms
//  - We try to not depend on IP uniqueness, but this is also requirement for LOCAL-vm communication
//  - We are the only user of ct_zone other than 0
//
// Flows with "VM" in them are guest nic-specific flows
//
// Table 0
// 40000 ipv6,actions=drop
// 30050 ip,nw_dst=K8S_CIDR,actions=mod_LOCAL
// 30040 ip,nw_src=K8S_CIDR,nw_dst=IP_VM,in_port=LOCAL,actions=mod_dl_dst:MAC_VM,output:PORT_VM
// 29310 in_port=LOCAL,metaserver_req,actions=normal
// 29300 metaserver_req,actions=mod_metaserver
// 29200 metaserver_resp_VM_IP,actions=mod_metaserver_PORT_VM
// 28400 in_port=PORT_VM,dhcp_req,actions=LOCAL
// 28300 in_port=LOCAL,dhcp_resp,actions=PORT_VM
//
// 27300 in_port=LOCAL,dl_dst=MAC_VM,ip,actions=load_dst_VM_ZONE,ct(zone=dst_VM_ZONE,table=sec_CT)
// 27200 in_port=LOCAL,actions=normal
//
// 26900 in_port=PORT_PHY,dl_dst=MAC_PHY,actions=normal
// 26870 in_port=PORT_PHY,dl_dst=MAC_VM,dl_vlan=VLAN_VM,ip,{SrcIpCheck?nw_dst=IP_VM},actions=load_dst_VM_ZONE,ct(zone=dst_VM_ZONE,table=sec_CT)
// 26860 in_port=PORT_PHY,dl_dst=MAC_VM,dl_vlan=VLAN_VM,ip,actions=drop                 SrcIpCheck
// 26700 in_port=PORT_PHY,dl_dst=MAC_VM,dl_vlan=VLAN_VM,actions=normal
//
// 25870 in_port=PORT_VM,dl_src=MAC_VM,ip,{SrcIpCheck?nw_src=IP_VM},actions=load_src_VM_ZONE,load_VM_BIT,ct(zone=src_VM_ZONE,table=sec_CT)
// 25860 in_port=PORT_VM,dl_src=MAC_VM,ip,actions=drop                                  SrcIpCheck
// 25770 in_port=PORT_VM,arp,dl_src=MAC_VM,arp_sha=MAC_VM,arp_spa=IP_VM,actions=normal  !allow_switch_vms
// 25760 in_port=PORT_VM,arp,actions=drop                                               !allow_switch_vms
// 25600 in_port=PORT_VM,dl_src=MAC_VM,actions=normal
//
// 24770 dl_dst=MAC_VM,ip,{SrcIpCheck?nw_dst=IP_VM},actions=load_dst_VM_ZONE,ct(zone=dst_VM_ZONE,table=sec_CT)
// 24760 dl_dst=MAC_VM,ip,actions=drop                                              SrcIpCheck
// 24670 in_port=PORT_VM,{ allow_switch_vms},actions=normal
// 24660 in_port=PORT_VM,{!allow_switch_vms},actions=drop
//
// 23700 in_port=PORT_PHY,{ allow_switch_vms},actions=normal
// 23600 in_port=PORT_PHY,{!allow_switch_vms},dl_dst=01:00:00:00:00:00/01:00:00:00:00:00,actions=normal
// 23500 in_port=PORT_PHY,{!allow_switch_vms},actions=drop
//
// Table 1 sec_CT
//  7900 ip,ct_state=+trk+inv,actions=drop
//  7800 ip,ct_state=+trk+new,{{!in_port_vm}},actions=resubmit(,sec_IN)
//  7700 ip,ct_state=+trk+new,{{ in_port_vm}},actions=resubmit(,sec_OUT)
//  7600 ip,actions=resubmit(,sec_CT_OkayEd)
//
// Table 2 sec_OUT
// 40000 in_port=PORT_VM,match_allow,actions=resubmit(,sec_IN)
//   ... in_port=PORT_VM,match_deny,actions=drop
//
// Table 3 sec_IN
// 40000 dl_dst=MAC_VM,match_allow,actions=load_dst_VM_ZONE,resubmit(,sec_CT_commit)
//   ... dl_dst=MAC_VM,match_deny,actions=drop
//    30 ip,actions=ct(commit,zone=reg0_ZONE),normal
//
// Table 4 sec_CT_OkayEd
//  5600 ip,dl_dst=MAC_VM,actions=load_dst_VM_ZONE,resubmit(,sec_CT_commit)
//  5500 ip,actions=ct(commit,zone=reg0_ZONE),normal
//
// Table 5 sec_CT_commit
//    20 ip,{!in_port_vm},actions=ct(commit,zone=dst_VM_ZONE),normal
//    10 ip,{ in_port_vm},actions=commit(commit,zone=src_VM_ZONE),ct(commit,zone=dst_VM_ZONE),normal
//     0 drop
