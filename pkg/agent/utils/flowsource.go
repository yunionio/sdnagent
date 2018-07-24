package utils

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/digitalocean/go-openvswitch/ovs"

	"yunion.io/yunioncloud/pkg/log"
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
		"K8SCidr":      h.K8SCidr,
		"MAC":          h.MAC,
		"IP":           h.IP,
		"MetadataPort": h.MetadataPort,
		"PortNoPhy":    ps.PortID,
	}
	T := t(m)
	flows := []*ovs.Flow{
		F(0, 40000, "ipv6", "drop"),
	}
	if h.K8SCidr != nil {
		flows = append(flows, F(0, 30050,
			T("ip,nw_dst={{.K8SCidr}}"),
			T("mod_dl_dst:{{.MAC}},local")))
	}
	flows = append(flows,
		F(0, 29300, "tcp,nw_dst=169.254.169.254,tp_dst=80",
			T("mod_dl_dst:{{.MAC}},mod_nw_dst:{{.IP}},mod_tp_dst:{{.MetadataPort}},LOCAL")),
		F(0, 27200, "in_port=LOCAL", "normal"),
		F(0, 27100, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}}"), "normal"),
		F(0, 25700, T("in_port={{.PortNoPhy}},dl_dst=01:00:00:00:00:00/01:00:00:00:00:00"), "normal"),
		F(0, 25600, T("in_port={{.PortNoPhy}}"), "drop"),
	)
	return map[string][]*ovs.Flow{h.Bridge: flows}, nil
}

func (g *Guest) Who() string {
	return g.Id
}

func (g *Guest) FlowsMap() (map[string][]*ovs.Flow, error) {
	r := map[string][]*ovs.Flow{}
	allGood := true
	for _, nic := range g.NICs {
		ofCli := ovs.New().OpenFlow
		ps, err := ofCli.DumpPort(nic.Bridge, nic.IfnameHost)
		if err != nil {
			log.Warningf("fetch guest %s port_no %s,%s failed: %s",
				g.Id, nic.Bridge, nic.IfnameHost, err)
			allGood = false
			continue
		}
		portNo := ps.PortID
		hcn := g.HostConfig.HostNetworkConfig(nic.Bridge)
		if hcn == nil {
			log.Warningf("guest %s port %s: no host network config for %s",
				g.Id, nic.IfnameHost, nic.Bridge)
			allGood = false
			continue
		}
		ps, err = ofCli.DumpPort(nic.Bridge, hcn.Ifname)
		if err != nil {
			log.Warningf("fetch phy port_no of %s,%s failed: %s",
				nic.Bridge, hcn.Ifname, err)
			allGood = false
			continue
		}
		portNoPhy := ps.PortID
		m := nic.Map()
		m["MetadataPort"] = g.HostConfig.Port
		m["PortNo"] = portNo
		m["PortNoPhy"] = portNoPhy
		T := t(m)
		if nic.VLAN > 1 {
			m["_dl_vlan"] = T("dl_vlan={{.VLAN}}")
			m["_ct_zone"] = 1000 + nic.VLAN
		} else {
			m["_dl_vlan"] = T("vlan_tci={{.VLANTci}}")
			m["_ct_zone"] = 1000
		}
		flows := []*ovs.Flow{
			F(0, 29200,
				T("in_port=LOCAL,tcp,nw_dst={{.IP}},tp_src={{.MetadataPort}}"),
				T("mod_dl_dst:{{.MAC}},mod_nw_src:169.254.169.254,mod_tp_src:80,output:{{.PortNo}}")),
			F(0, 28400, T("in_port={{.PortNo}},udp,tp_src=68,tp_dst=67"), "local"),
			F(0, 28300, T("in_port=LOCAL,dl_dst={{.MAC}}"), T("output:{{.PortNo}}")),
			F(0, 25900, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}}"), "normal"),
			F(0, 25800, T("in_port={{.PortNo}}"), "normal"),
		}
		flows = append(flows, g.SecurityRules.Flows(m)...)
		if fs, ok := r[nic.Bridge]; ok {
			flows = append(fs, flows...)
		}
		r[nic.Bridge] = flows
	}
	if !allGood {
		return r, fmt.Errorf("guest port is not ready yet")
	}
	return r, nil
}

func (sr *SecurityRules) Flows(data map[string]interface{}) []*ovs.Flow {
	T := t(data)
	data["_in_port_phy"] = "reg0=0x1/0x1"
	data["_in_port_vm"] = "reg0=0x0/0x1"
	loadReg0BitPhy := "load:0x1->NXM_NX_REG0[0]"
	loadReg0BitVm := "load:0->NXM_NX_REG0[0]" // "0->" is important, not "0x0->"

	flows := []*ovs.Flow{}
	// table 0
	// table 1 sec_CT
	flows = append(flows,
		F(0, 26900, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ip,ct_state=-trk"),
			loadReg0BitPhy+T(",ct(table=1,zone={{._ct_zone}})")),
		F(0, 26800, T("in_port={{.PortNo}},ip,ct_state=-trk"),
			loadReg0BitVm+T(",ct(table=1,zone={{._ct_zone}})")),
		// ct_state= flags order matters
		F(1, 7900, T("ip,ct_zone={{._ct_zone}},ct_state=+inv+trk"), "drop"),
		F(1, 7800, T("ip,ct_zone={{._ct_zone}},ct_state=+new+trk,{{._in_port_phy}}"), "resubmit(,3)"),
		F(1, 7700, T("ip,ct_zone={{._ct_zone}},ct_state=+new+trk,{{._in_port_vm}}"), "resubmit(,2)"),
		F(1, 7600, T("ip,ct_zone={{._ct_zone}}"), "normal"),
	)

	// table sec_CT_OUT
	//
	// out is must to allow resubmit(,sec_CT_IN)
	prioOut := 40000
	matchOut := T("in_port={{.PortNo}},ip")
	for _, r := range sr.outRules {
		if prioOut <= 20 {
			log.Errorf("%s: %q generated too out rules",
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
	// NOTE the rule is mainly for debugging purposes: counting packets
	// dropped for no matching in the out pipeline
	flows = append(flows, F(2, 20, matchOut, "drop"))

	// table sec_CT_IN
	//
	// in is also a must to forward packets from sec_CT_OUT
	//
	// NOTE assume MAC is unique across the platform
	prioIn := 40000
	matchIn := T("dl_dst={{.MAC}}")
	actionAllowIn := T("ct(commit,zone={{._ct_zone}}),normal")
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
	// NOTE when this rule matches, it means no in vm secgroup rules blocks
	// or accepts the packet in question, which means it's free to go.
	//
	// Indeed this is needed for accessing network entities other than
	// those locally managed.  This also means traffics from phy port will
	// be allowed if no ingress rule matches it
	flows = append(flows, F(3, 30, "ip", actionAllowIn))
	return flows
}

// NOTE: KEEP THIS IN SYNC WITH CODE ABOVE
//
// Assumptions
//
//  - MAC is unique, IP is not
//  - We are the only user of ct_zone other than 0
//
// Flows with "VM" in them are guest nic-specific flows
//
// Table 0
// 40000 ipv6,actions=drop
// 30050 ip,nw_dst=K8S_CIDR,actions=mod_LOCAL
// 29300 metaserver_req,actions=mod_metaserver
// 29200 metaserver_resp_VM_IP,actions=mod_metaserver_PORT_VM
// 28400 in_port=PORT_VM,dhcp_req,actions=LOCAL
// 28300 in_port=LOCAL,dl_dst=MAC_VM,actions=output:PORT_VM
// 27200 in_port=LOCAL,actions=normal
// 27100 in_port=PORT_PHY,dl_dst=MAC_PHY,actions=normal
//
// 26900 in_port=PORT_PHY,dl_dst=MAC_VM,dl_vlan=VLAN_VM,ip,ct_state=-trk,actions=load_ZONE,load_PHY_BIT,ct(zone=ZONE,table=sec_CT)
// 26800 in_port=PORT_VM,ip,ct_state=-trk,actions=load_ZONE,load_VM_BIT,ct(zone=ZONE,table=sec_CT)
// 25900 in_port=PORT_PHY,dl_dst=MAC_VM,dl_vlan=VLAN_VM,actions=normal
// 25800 in_port=PORT_VM,actions=normal
// 25700 in_port=PORT_PHY,dl_dst=01:00:00:00:00:00/01:00:00:00:00:00,actions=normal
// 25600 in_port=PORT_PHY,actions=drop
//
// Table 1 sec_CT
//  7900 ip,ct_zone=ZONE,ct_state=+trk+inv,actions=drop
//  7800 ip,ct_zone=ZONE,ct_state=+trk+new,{{reg0_phy_set}},actions=resubmit(,sec_IN)
//  7700 ip,ct_zone=ZONE,ct_state=+trk+new,{{reg0_vm_set}},actions=resubmit(,sec_OUT)
//  7600 ip,ct_zone=ZONE,actions=normal
//
// Table 2 sec_OUT
// 40000 in_port=PORT_VM,match_allow,actions=resubmit(,sec_IN)
//   ... in_port=PORT_VM,match-deny,actions=drop
//    20 in_port=PORT_VM,,actions=drop
//
// Table 3 sec_IN
// 40000 dl_dst=MAC_VM,match_allow,actions=ct(zone=ZONE,commit),normal
//   ... dl_dst=MAC_VM,match_deny,,actions=drop
//    30 dl_src=MAC_VM,actions=normal
//
// grep -oE '\<F\([0-9].*' pkg/agent/utils/flowsource.go  | cat
