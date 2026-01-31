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
	"net"
	"strings"
	"text/template"

	"github.com/digitalocean/go-openvswitch/ovs"
	"github.com/vishvananda/netlink"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"
	"yunion.io/x/pkg/util/netutils"
	"yunion.io/x/pkg/util/stringutils"

	"yunion.io/x/onecloud/pkg/util/iproute2"
	"yunion.io/x/onecloud/pkg/util/netutils2"
)

type FlowSource interface {
	Who() string
	FlowsMap() (map[string][]*ovs.Flow, error)
}

func F(table, priority int, matches, actions string) *ovs.Flow {
	txt := fmt.Sprintf("table=%d,priority=%d,%s,actions=%s", table, priority, matches, actions)
	// log.Debugln(txt)
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

func FakeArpRespActions(macStr string) string {
	hexMac := "0x" + strings.TrimLeft(strings.ReplaceAll(macStr, ":", ""), "0")

	mdArpRespActions := []string{
		"move:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[]",
		fmt.Sprintf("load:%s->NXM_OF_ETH_SRC[]", hexMac),
		"load:0x2->NXM_OF_ARP_OP[]",
		fmt.Sprintf("load:%s->NXM_NX_ARP_SHA[]", hexMac),
		"move:NXM_OF_ARP_TPA[]->NXM_OF_ARP_SPA[]",
		"move:NXM_NX_ARP_SHA[]->NXM_NX_ARP_THA[]",
		"move:NXM_OF_ARP_SPA[]->NXM_OF_ARP_TPA[]",
		"in_port",
	}

	return strings.Join(mdArpRespActions, ",")
}

func (h *HostLocal) EnsureFakeLocalMetadataRoute() error {
	prefix, _, _ := h.fakeMdSrcIpMac(0)
	nic := netutils2.NewNetInterface(h.Bridge)
	routes := nic.GetRouteSpecs()
	find := false
	for i := range routes {
		if routes[i].Dst.String() == prefix {
			find = true
			break
		}
	}
	if !find {
		rt := iproute2.NewRoute(h.Bridge)
		err := rt.AddByCidr(prefix, "").Err()
		if err != nil {
			return errors.Wrap(err, "AddRouteByCidr")
		}
	}
	return nil
}

func (h *HostLocal) EnsureFakeLocalMetadataRoute6() error {
	prefix, _, _ := h.fakeMdSrcIp6Mac(0, "")
	nic := netutils2.NewNetInterface(h.Bridge)
	routes := nic.GetRouteSpecs()
	find := false
	for i := range routes {
		if routes[i].Dst.String() == prefix {
			find = true
			break
		}
	}
	if !find {
		rt := iproute2.NewRoute(h.Bridge)
		err := rt.AddByCidr(prefix, "").Err()
		if err != nil {
			return errors.Wrap(err, "AddRouteByCidr")
		}
	}
	return nil
}

func (h *HostLocal) FlowsMap() (map[string][]*ovs.Flow, error) {
	ps, err := DumpPort(h.Bridge, h.Ifname)
	if err != nil {
		return nil, err
	}
	m := map[string]interface{}{
		"MetadataPort": h.metadataPort,
		"MAC":          h.MAC,
		"PortNoPhy":    ps.PortID,
	}
	if h.IP != nil {
		m["IP"] = h.IP
	}
	if h.IP6 != nil {
		m["IP6"] = h.IP6
	}
	if h.IP6Local != nil {
		m["IP6Local"] = h.IP6Local
	}
	T := t(m)
	flows := []*ovs.Flow{
		// allow ipv6
		// F(0, 40000, "ipv6", "drop"),
	}
	flows = append(flows,
		F(0, 27200, "in_port=LOCAL", "normal"),
		F(0, 26900, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}}"), "normal"),
	)
	if h.IP != nil {
		flows = append(flows,
			// drop arp request from outside to metadata IPv4 address
			F(0, 29312, T("in_port={{.PortNoPhy}},arp,arp_op=1,arp_tpa=169.254.169.254"), "drop"),
			// arp response to metadata IPv4 address for guest
			F(0, 29311, "arp,arp_op=1,arp_tpa=169.254.169.254", FakeArpRespActions(h.MAC.String())),
		)
		// direct all ipv4 metadata response to table 12
		prefix, _, mac := h.fakeMdSrcIpMac(0)
		flows = append(flows,
			F(0, 29310, fmt.Sprintf("in_port=LOCAL,tcp,dl_dst=%s,nw_dst=%s,tp_src=%d", mac, prefix, h.metadataPort), "resubmit(,12)"),
		)
	}
	if h.IP6 != nil {
		// drop nbp solicitation from outside to IPv6 metadata  address
		for i := range h.HostConfig.MetadataServerIp6s {
			metaSrvIp6 := h.HostConfig.MetadataServerIp6s[i]
			flows = append(flows,
				F(0, 40013+i, T(fmt.Sprintf("in_port={{.PortNoPhy}},ipv6,icmp6,icmp_type=135,nd_target=%s", metaSrvIp6)), "drop"),
			)
		}
		flows = append(flows,
			// allow any IPv6 link local multicast
			F(0, 40000, T("dl_dst=01:00:00:00:00:00/01:00:00:00:00:00,ipv6,icmp6,ipv6_dst=ff02::/64"), "normal"),
			// allow ipv6 nb solicit and advertise from outside
			F(0, 30001, T("in_port={{.PortNoPhy}},ipv6,icmp6,icmp_type=135"), "normal"),
			F(0, 30002, T("in_port={{.PortNoPhy}},ipv6,icmp6,icmp_type=136"), "normal"),
			// allow ipv6 nb solicit and advertise from local
			F(0, 30003, T("in_port=LOCAL,ipv6,icmp6,icmp_type=135"), "normal"),
			F(0, 30004, T("in_port=LOCAL,ipv6,icmp6,icmp_type=136"), "normal"),
			// hijack ipv6 router solicitation and advertisement from outside to local, priority should be higher than 40000
			F(0, 40001, T("in_port={{.PortNoPhy}},ipv6,icmp6,icmp_type=133"), T("mod_dl_dst:{{.MAC}},local")),
			F(0, 40002, T("in_port={{.PortNoPhy}},ipv6,icmp6,icmp_type=134"), T("mod_dl_dst:{{.MAC}},local")),
		)

		prefix, _, mac := h.fakeMdSrcIp6Mac(0, "")
		flows = append(flows,
			F(0, 29310, fmt.Sprintf("in_port=LOCAL,ipv6,tcp6,dl_dst=%s,ipv6_dst=%s,tp_src=%d", mac, prefix, h.metadataPort), "resubmit(,12)"),
			F(0, 40050, fmt.Sprintf("ipv6,icmp6,icmp_type=136,dl_dst=%s,ipv6_dst=%s", mac, prefix), "resubmit(,12)"),
		)
	}
	{
		flows = append(flows,
			// 未经过conntrack的，先走一下conntrack
			F(9, 40001, "tcp,ct_state=-trk", "ct(table=9)"),
			F(9, 40000, "udp,ct_state=-trk", "ct(table=9)"),
			// 如果是从虚拟机发出的，新的报文，则应该丢弃
			F(9, 31001, "reg0=0x4,tcp,ct_state=+new+trk", "drop"),
			F(9, 31000, "reg0=0x4,udp,ct_state=+new+trk", "drop"),
			// 如果从外部或者本地请求的新的报文，则commit，并正常转发
			F(9, 30001, "tcp,ct_state=+new+trk", "ct(commit,exec(move:NXM_NX_REG0[]->NXM_NX_CT_MARK[])),normal"),
			F(9, 30000, "udp,ct_state=+new+trk", "ct(commit,exec(move:NXM_NX_REG0[]->NXM_NX_CT_MARK[])),normal"),
			// 如果是从虚拟机发出的，且ct_mark=2的，是直接请求流量的响应，需正常处理
			F(9, 20003, "reg0=0x4,tcp,ct_state=+est+trk,ct_mark=0x2", "normal"),
			F(9, 20002, "reg0=0x4,udp,ct_state=+est+trk,ct_mark=0x2", "normal"),
			// 如果是从虚拟机发出的，且ct_mark=1的，是外部做了port_mapping请求的响应，需要转到table=10 UNDO DNAT
			F(9, 20001, "reg0=0x4,tcp,ct_state=+est+trk,ct_mark=0x1", "resubmit(,10)"),
			F(9, 20000, "reg0=0x4,udp,ct_state=+est+trk,ct_mark=0x1", "resubmit(,10)"),
			// 其他的是从外部进入的，正常转发
			F(9, 10001, "tcp,ct_state=+est+trk", "normal"),
			F(9, 10000, "udp,ct_state=+est+trk", "normal"),
			// 其他 drop
			F(9, 1000, "", "drop"),
		)
	}
	{
		// prevent hostlocal IPs leaking outside of host
		for i := range h.HostLocalNets {
			netConf := h.HostLocalNets[i]
			ip4, _ := netutils.NewIPV4Addr(netConf.GuestIpStart)
			addrMask := fmt.Sprintf("%s/%d", ip4.NetAddr(int8(netConf.GuestIpMask)).String(), netConf.GuestIpMask)
			flows = append(flows,
				F(0, 39000, T(fmt.Sprintf("arp,arp_tpa=%s", addrMask)), "drop"),
				F(0, 39001, T(fmt.Sprintf("arp,arp_spa=%s", addrMask)), "drop"),
			)
		}
	}
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

func (g *Guest) getMetadataInfo(nic *GuestNIC) (mdIP, mdIP6 string, mdMAC string, mdPortNo int, useLOCAL bool, err error) {
	useLOCAL = true
	ipStr := nic.IP
	if len(ipStr) == 0 {
		ipStr = nic.IP6
	}
	route, err := RouteLookup(ipStr)
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

	{
		var (
			p      *ovs.PortStats
			addrs  []netlink.Addr
			addrs6 []netlink.Addr
		)
		{
			attrs := linkPeer.Attrs()
			p, err = DumpPort(nic.Bridge, attrs.Name)
			if err != nil {
				var masterLink netlink.Link
				masterLink, err = netlink.LinkByIndex(attrs.MasterIndex)
				if err != nil {
					return
				}
				p, err = DumpPort(nic.Bridge, masterLink.Attrs().Name)
				if err != nil {
					return
				}
			}
		}
		addrs, err = netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			return
		}

		addrs6, err = netlink.AddrList(link, netlink.FAMILY_V6)
		if err != nil {
			return
		}
		if len(addrs) == 0 && len(addrs6) == 0 {
			return
		}
		err = nil
		useLOCAL = false
		if len(addrs) > 0 {
			mdIP = addrs[0].IP.String()
		}
		if len(addrs6) > 0 {
			for _, addr := range addrs6 {
				if addr.IP.IsLinkLocalUnicast() {
					mdIP6 = addr.IP.String()
					break
				}
			}
		}
		mdMAC = link.Attrs().HardwareAddr.String()
		mdPortNo = p.PortID
	}
	return
}

func isNicHostLocal(hcn *HostConfigNetwork, nic *GuestNIC) bool {
	for i := range hcn.HostLocalNets {
		if nic.NetId == hcn.HostLocalNets[i].Id {
			return true
		}
	}
	return false
}

func macToHex(b interface{}) string {
	mac, _ := net.ParseMAC(b.(string))
	return stringutils.Bytes2Str(mac)
}

func ip6ToHex(ip6str string) string {
	ip6 := net.ParseIP(ip6str)
	return stringutils.Bytes2Str(ip6[0:16])
}

func (g *Guest) FlowsMapForNic(nic *GuestNIC) ([]*ovs.Flow, error) {
	if nic.PortNo <= 0 {
		return nil, errors.Wrap(errors.ErrInvalidStatus, "nic.PortNo <= 0")
	}
	hcn := g.HostConfig.HostNetworkConfig(nic.Bridge)
	if hcn == nil {
		log.Warningf("guest %s port %s: no host network config for %s",
			g.Id, nic.IfnameHost, nic.Bridge)
		return nil, errors.Wrapf(errors.ErrInvalidStatus, "guest %s port %s: no host network config for %s", g.Id, nic.IfnameHost, nic.Bridge)
	}
	ps, err := DumpPort(nic.Bridge, hcn.Ifname)
	if err != nil {
		log.Warningf("fetch phy port_no of %s,%s failed: %s",
			nic.Bridge, hcn.Ifname, err)
		return nil, errors.Wrapf(err, "fetch phy port_no of %s,%s failed", nic.Bridge, hcn.Ifname)
	}
	portNoPhy := ps.PortID
	m := nic.Map()
	m["DHCPServerPort"] = g.HostConfig.DhcpServerPort
	m["DHCPServerPort6"] = g.HostConfig.Dhcp6ServerPort
	m["PortNoPhy"] = portNoPhy
	{
		mac, err := hcn.MAC()
		if err != nil {
			log.Warningf("host network find ip mac: %v", err)
			return nil, errors.Wrap(err, "host network find ip mac")
		}
		if hcn.IP != nil {
			m["IPPhy"] = hcn.IP.String()
		}
		if hcn.IP6Local != nil {
			// ipv6 use link local address to servce metadata service
			m["IP6Phy"] = hcn.IP6Local.String()
		}
		m["MACPhy"] = mac.String()
	}
	{
		var mdPortAction string
		var mdInPort string
		mdIP, mdIP6, mdMAC, mdPortNo, useLOCAL, err := g.getMetadataInfo(nic)
		log.Debugf("getMetadataInfo %s %s: mdIP=%s mdMAC=%s mdPortNo=%d useLOCAL=%v", nic.IP, nic.Bridge, mdIP, mdMAC, mdPortNo, useLOCAL)
		if useLOCAL {
			if err != nil {
				log.Warningf("find metadata: %v", err)
			}
			if m["IPPhy"] != nil {
				mdIP = m["IPPhy"].(string)
			}
			if m["IP6Phy"] != nil {
				mdIP6 = m["IP6Phy"].(string)
			}
			mdMAC = m["MACPhy"].(string)
			// mdPortNo = portNoPhy
			mdInPort = "LOCAL"
			mdPortAction = "LOCAL"
		} else {
			mdInPort = fmt.Sprintf("%d", mdPortNo)
			mdPortAction = fmt.Sprintf("output:%d", mdPortNo)
		}
		hostLocal := findHostLocalByBridge(hcn.Bridge)
		if hostLocal != nil {
			m["MetadataServerPort"] = hostLocal.metadataPort
		}
		if mdIP != "" {
			m["MetadataServerIP"] = mdIP
		}
		if mdIP6 != "" {
			m["MetadataServerIP6"] = mdIP6
		}
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
	if mdPort, ok := m["MetadataServerPort"]; ok {
		if _, ok := m["MetadataServerIP"]; ok && nic.EnableIPv4() {
			// enable IPv4 metadata service
			_, fakeSrcIp, fakeSrcMac := fakeMdSrcIpMac(m["MetadataServerIP"].(string), m["MetadataServerMAC"].(string), nic.PortNo)
			m["MetadataClientFakeIP"] = fakeSrcIp
			m["MetadataClientFakeMAC"] = fakeSrcMac
			// rule from host metadata to VM was learnd
			learnStr := fmt.Sprintf("learn(table=12,priority=10000,idle_timeout=30,in_port=LOCAL,dl_type=0x0800,nw_proto=6,nw_dst=%s,tcp_src=%d,load:NXM_OF_ETH_DST[]->NXM_OF_ETH_SRC[],load:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],load:NXM_OF_IP_DST[]->NXM_OF_IP_SRC[],load:NXM_OF_IP_SRC[]->NXM_OF_IP_DST[],load:0x50->NXM_OF_TCP_SRC[],output:NXM_OF_IN_PORT[]),", fakeSrcIp, mdPort)
			flows = append(flows,
				// arp response to fake metadata client IP and MAC
				F(0, 29301, fmt.Sprintf("in_port=LOCAL,arp,arp_op=1,arp_tpa=%s", fakeSrcIp), FakeArpRespActions(fakeSrcMac)),
				// from VM to host metadata
				F(0, 29300, T("in_port={{.PortNo}},tcp,nw_dst=169.254.169.254,tp_dst=80"),
					learnStr+T("mod_dl_src:{{.MetadataClientFakeMAC}},mod_dl_dst:{{.MetadataServerMAC}},mod_nw_src:{{.MetadataClientFakeIP}},mod_nw_dst:{{.MetadataServerIP}},mod_tp_dst:{{.MetadataServerPort}},{{.MetadataPortAction}}")),
			)
		}
		if mIP6Str, ok := m["MetadataServerIP6"]; ok && nic.EnableIPv6() {
			// enable IPv6 metadata service
			mIP6 := net.ParseIP(mIP6Str.(string))
			mIP6McastIP := netutils2.IP2SolicitMcastIP(mIP6).String()
			mIP6McastMac := netutils2.IP2SolicitMcastMac(mIP6).String()

			// ndp solicitation from VM to metadata server
			for i := range g.HostConfig.MetadataServerIp6s {
				metaSrvIp6 := g.HostConfig.MetadataServerIp6s[i]
				_, fakeVmSrcIp6, fakeVmSrcMac6 := fakeMdSrcIp6Mac(m["MetadataServerIP6"].(string), m["MetadataServerMAC"].(string), nic.PortNo, metaSrvIp6)

				log.Debugf(" g.HostConfig.MetadataServerIp6s: %s metaSrcIP6: %s, fakeVmSrcIp6: %s, fakeVmSrcMac6: %s", jsonutils.Marshal(g.HostConfig.MetadataServerIp6s), metaSrvIp6, fakeVmSrcIp6, fakeVmSrcMac6)

				learnMdNdpAdvStr := fmt.Sprintf("learn(table=12,priority=20000,idle_timeout=30,in_port=LOCAL,dl_type=0x86dd,nw_proto=58,icmpv6_type=136,dl_dst=%s,ipv6_dst=%s,nd_target={{.MetadataServerIP6}},load:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],load:0x%s->NXM_OF_ETH_SRC[],load:NXM_NX_IPV6_SRC[]->NXM_NX_IPV6_DST[],load:NXM_NX_ND_TARGET[]->NXM_NX_IPV6_SRC[],load:NXM_NX_ND_TARGET[]->NXM_NX_ND_TARGET[],load:0x%s->NXM_NX_ND_TLL[],output:NXM_OF_IN_PORT[]),", fakeVmSrcMac6, fakeVmSrcIp6, macToHex(m["MetadataServerMAC"]), macToHex(m["MetadataServerMAC"]))
				learnHttpStr := fmt.Sprintf("learn(table=12,priority=10000,idle_timeout=30,in_port=LOCAL,dl_type=0x86dd,nw_proto=6,ipv6_dst=%s,tcp_src=%d,load:NXM_OF_ETH_DST[]->NXM_OF_ETH_SRC[],load:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],load:NXM_NX_IPV6_DST[]->NXM_NX_IPV6_SRC[],load:NXM_NX_IPV6_SRC[]->NXM_NX_IPV6_DST[],load:0x50->NXM_OF_TCP_SRC[],output:NXM_OF_IN_PORT[]),", fakeVmSrcIp6, mdPort)

				fakeVmSrcIp6Str := ip6ToHex(fakeVmSrcIp6)
				mIP6McastIPStr := ip6ToHex(mIP6McastIP)
				mIP6HexStr := ip6ToHex(mIP6Str.(string))
				metaSrvIp6Str := ip6ToHex(metaSrvIp6)
				vmIP6Str := ip6ToHex(m["IP6"].(string))
				vmIP6McastStr := ip6ToHex(m["IP6McastIP"].(string))

				flows = append(flows,
					// ndp solication from VM to metadata server
					F(0, 40011+i,
						T("in_port={{.PortNo}},ipv6,icmp6,icmp_type=135,nd_target="+metaSrvIp6),
						T(learnMdNdpAdvStr+fmt.Sprintf("mod_dl_src:%s,mod_dl_dst:%s,load:0x%s->NXM_NX_IPV6_SRC[0..127],load:0x%s->NXM_NX_IPV6_DST[0..127],load:0x%s->NXM_NX_ND_TARGET[0..127],load:0x%s->NXM_NX_ND_SLL[],local", fakeVmSrcMac6, mIP6McastMac, fakeVmSrcIp6Str, mIP6McastIPStr, mIP6HexStr, macToHex(fakeVmSrcMac6)))),
					// ndp solication from metadata server to VM
					F(0, 40012+i,
						T(fmt.Sprintf("in_port=LOCAL,ipv6,icmp6,icmp_type=135,nd_target=%s", fakeVmSrcIp6)),
						T(fmt.Sprintf("mod_dl_dst:{{.IP6McastMac}},load:0x%s->NXM_NX_IPV6_SRC[0..127],load:0x%s->NXM_NX_IPV6_DST[0..127],load:0x%s->NXM_NX_ND_TARGET[0..127],output:{{.PortNo}}", metaSrvIp6Str, vmIP6McastStr, vmIP6Str))),
					// ndp advertisement from VM to metadata server
					F(0, 40013+i,
						T(fmt.Sprintf("in_port={{.PortNo}},dl_src={{.MAC}},dl_dst={{.MetadataServerMAC}},ipv6_src={{.IP6}},ipv6_dst=%s,icmp6,icmp_type=136,nd_target={{.IP6}}", metaSrvIp6)),
						T(fmt.Sprintf("mod_dl_src:%s,load:0x%s->NXM_NX_IPV6_SRC[0..127],load:0x%s->NXM_NX_IPV6_DST[0..127],load:0x%s->NXM_NX_ND_TARGET[0..127],load:0x%s->NXM_NX_ND_TLL[],local", fakeVmSrcMac6, fakeVmSrcIp6Str, mIP6HexStr, fakeVmSrcIp6Str, macToHex(fakeVmSrcMac6)))),
					// from VM to host metadata
					F(0, 29301+i,
						T(fmt.Sprintf("in_port={{.PortNo}},ipv6,tcp6,ipv6_dst=%s,tp_dst=80", metaSrvIp6)),
						learnHttpStr+T(fmt.Sprintf("mod_dl_src:%s,mod_dl_dst:{{.MetadataServerMAC}},load:0x%s->NXM_NX_IPV6_SRC[0..127],load:0x%s->NXM_NX_IPV6_DST[0..127],mod_tp_dst:{{.MetadataServerPort}},{{.MetadataPortAction}}", fakeVmSrcMac6, fakeVmSrcIp6Str, mIP6HexStr))),
				)
			}
		}
	}
	flows = append(flows,
		// dhcpv4 from VM to host
		F(0, 28400, T("in_port={{.PortNo}},ip,udp,tp_src=68,tp_dst=67"), T("mod_tp_dst:{{.DHCPServerPort}},local")),
		// dhcpv4 from host to VM
		F(0, 28300, T("in_port=LOCAL,dl_dst={{.MAC}},ip,udp,tp_src={{.DHCPServerPort}},tp_dst=68"), T("mod_tp_src:67,output:{{.PortNo}}")),
		// dhcpv6 from VM to host
		F(0, 28400, T("in_port={{.PortNo}},ipv6,udp6,tp_src=546,tp_dst=547"), T("mod_dl_dst:{{.MACPhy}},mod_tp_dst:{{.DHCPServerPort6}},local")),
		// dhcpv6 from host to VM
		F(0, 28300, T("in_port=LOCAL,dl_dst={{.MAC}},ipv6,udp6,tp_src={{.DHCPServerPort6}},tp_dst=546"), T("mod_tp_src:547,output:{{.PortNo}}")),
		// ra solicitation from VM to host
		F(0, 28400, T("in_port={{.PortNo}},ipv6,icmp6,icmp_type=133"), T("mod_dl_dst:{{.MACPhy}},local")),
		// ra advertisement from host to VM
		F(0, 28300, T("in_port=LOCAL,dl_dst={{.MAC}},ipv6,icmp6,icmp_type=134"), T("mod_dl_dst:33:33:00:00:00:01,output:{{.PortNo}}")),
		// allow any other traffic from host to vm
		F(0, 26700, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}}"), "normal"),
	)
	if !g.SrcMacCheck() {
		flows = append(flows, F(0, 24670, T("in_port={{.PortNo}}"), "normal"))
		if nic.EnableIPv6() {
			flows = append(flows,
				// allow nb solicite from VM port to outside
				F(0, 27770, T("in_port={{.PortNo}},ipv6,icmp6,icmp_type=135"), "normal"),
				// allow nb advertisement from VM port to outside
				F(0, 27770, T("in_port={{.PortNo}},ipv6,icmp6,icmp_type=136"), "normal"),
			)
		}
	} else {
		if !g.SrcIpCheck() {
			// allow any ARP from VM mac
			flows = append(flows,
				F(0, 27770, T("in_port={{.PortNo}},arp,dl_src={{.MAC}},arp_sha={{.MAC}}"), "normal"),
			)
			if nic.EnableIPv6() {
				flows = append(flows,
					// allow nb soliciate from VM src mac
					F(0, 27770, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6,icmp6,icmp_type=135"), "normal"),
					// allow nb advertisement from VM src mac
					F(0, 27770, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6,icmp6,icmp_type=136"), "normal"),
				)
			}
		} else {
			// allow arp from VM src IP
			if nic.EnableIPv4() {
				if isNicHostLocal(hcn, nic) {
					g.eachIP(m, func(T2 func(string) string) {
						flows = append(flows,
							F(0, 39011, T2(fmt.Sprintf("in_port=LOCAL,arp,arp_spa=%s,arp_tpa={{.IP}}", nic.Gateway)), FakeArpRespActions(nic.MAC)),
							F(0, 39010, T2(fmt.Sprintf("in_port={{.PortNo}},arp,dl_src={{.MAC}},arp_sha={{.MAC}},arp_spa={{.IP}},arp_tpa=%s", nic.Gateway)), FakeArpRespActions(hcn.mac.String())),
						)
					})
				} else {
					g.eachIP(m, func(T2 func(string) string) {
						flows = append(flows,
							F(0, 27770, T2("in_port={{.PortNo}},arp,dl_src={{.MAC}},arp_sha={{.MAC}},arp_spa={{.IP}}"), "normal"),
						)
					})
				}
			}
			if nic.EnableIPv6() {
				flows = append(flows,
					// allow nb solicitate from VM src IP to outside
					F(0, 27774, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6,ipv6_src={{.IP6}},icmp6,icmp_type=135"), "normal"),
					// allow nb solicitate from VM src IP to outside
					F(0, 27773, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6,ipv6_src={{.IP6LOCAL}},icmp6,icmp_type=135"), "normal"),
					// allow nb advert from VM src IP to outside
					F(0, 27772, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6,ipv6_src={{.IP6}},icmp6,icmp_type=136"), "normal"),
					// allow nb advert from VM src IP to outside
					F(0, 27771, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6,ipv6_src={{.IP6LOCAL}},icmp6,icmp_type=136"), "normal"),
				)
			}
		}

		if nic.EnableIPv4() && len(nic.PortMappings) > 0 {
			for _, pm := range nic.PortMappings {
				if len(pm.RemoteIps) == 0 {
					pm.RemoteIps = []string{"0.0.0.0/0"}
				}
				for _, remoteNet := range pm.RemoteIps {
					srcIpStr := ""
					if remoteNet != "0.0.0.0/0" {
						srcIpStr = fmt.Sprintf("nw_src=%s,", remoteNet)
					}
					protoNum := 6
					tpSrcField := "tcp_src"
					nxmTpSrc := "NXM_OF_TCP_SRC[]"
					nxmTpDst := "NXM_OF_TCP_DST[]"
					switch pm.Protocol {
					case "tcp":
						protoNum = 6
						tpSrcField = "tcp_src"
						nxmTpSrc = "NXM_OF_TCP_SRC[]"
						nxmTpDst = "NXM_OF_TCP_DST[]"
					case "udp":
						protoNum = 17
						tpSrcField = "udp_src"
						nxmTpSrc = "NXM_OF_UDP_SRC[]"
						nxmTpDst = "NXM_OF_UDP_DST[]"
					}
					learnStr := fmt.Sprintf("table=10,priority=10000,idle_timeout=30,in_port=%d,dl_type=0x0800,nw_proto=%d,%s=%d,load:NXM_OF_ETH_DST[]->NXM_OF_ETH_SRC[],load:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],load:NXM_OF_IP_DST[]->NXM_OF_IP_SRC[],load:%s->%s,output:NXM_OF_IN_PORT[]", nic.PortNo, protoNum, tpSrcField, pm.Port, nxmTpDst, nxmTpSrc)
					flows = append(flows,
						// 外部访问的流量, 需要DNAT, reg0=1
						F(0, 28200, fmt.Sprintf("in_port=%d,ip,%s%s,tp_dst=%d", portNoPhy, srcIpStr, pm.Protocol, *pm.HostPort), fmt.Sprintf("learn(%s),load:0x1->NXM_NX_REG0[],mod_dl_dst:%s,mod_nw_dst:%s,mod_tp_dst:%d,resubmit(,9)", learnStr, nic.MAC, nic.IP, pm.Port)),
						// 外部直接访问的流量，不需要DNAT，reg0=2
						F(0, 28200, fmt.Sprintf("in_port=%d,ip,%snw_dst=%s,%s,tp_dst=%d", portNoPhy, srcIpStr, nic.IP, pm.Protocol, pm.Port), "load:0x2->NXM_NX_REG0[],resubmit(,9)"),
					)
				}
				flows = append(flows,
					// 本地流量, 直接访问，不需要DNAT, reg0=2
					F(0, 28201, fmt.Sprintf("in_port=LOCAL,ip,nw_dst=%s,%s,tp_dst=%d", nic.IP, pm.Protocol, pm.Port), "load:0x2->NXM_NX_REG0[],resubmit(,9)"),
					// 返回流量, reg0=4
					F(0, 28202, fmt.Sprintf("in_port=%d,ip,%s,tp_src=%d", nic.PortNo, pm.Protocol, pm.Port), "load:0x4->NXM_NX_REG0[],resubmit(,9)"),
				)
			}
		}

		if g.HostConfig.DisableSecurityGroup {
			if !g.SrcIpCheck() {
				flows = append(flows,
					F(0, 26870, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ip"), "normal"),
					F(0, 25870, T("in_port={{.PortNo}},dl_src={{.MAC}},ip"), "normal"),
					F(0, 24770, T("dl_dst={{.MAC}},ip"), "normal"),
				)
				if nic.EnableIPv6() {
					flows = append(flows,
						F(0, 26870, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ipv6"), "normal"),
						F(0, 25870, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6"), "normal"),
						F(0, 24770, T("dl_dst={{.MAC}},ipv6"), "normal"),
					)
				}
			} else {
				if nic.EnableIPv4() {
					g.eachIP(m, func(T2 func(string) string) {
						// allow anythin from local ip
						flows = append(flows,
							F(0, 26870, T2("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ip,nw_dst={{.IP}}"), "normal"),
							F(0, 25870, T2("in_port={{.PortNo}},dl_src={{.MAC}},ip,nw_src={{.IP}}"), "normal"),
							F(0, 24770, T2("dl_dst={{.MAC}},ip,nw_dst={{.IP}}"), "normal"),
						)
					})
					// drop others
					flows = append(flows,
						F(0, 26860, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ip"), "drop"),
						F(0, 25860, T("in_port={{.PortNo}},dl_src={{.MAC}},ip"), "drop"),
						F(0, 24760, T("dl_dst={{.MAC}},ip"), "drop"),
					)
				}
				if nic.EnableIPv6() {
					flows = append(flows,
						// allow for ipv6 IP
						F(0, 26870, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ipv6,ipv6_dst={{.IP6}}"), "normal"),
						F(0, 25870, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6,ipv6_src={{.IP6}}"), "normal"),
						F(0, 24770, T("dl_dst={{.MAC}},ipv6,ipv6_dst={{.IP6}}"), "normal"),
						// allow for link local IP
						F(0, 26871, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ipv6,ipv6_dst={{.IP6LOCAL}}"), "normal"),
						F(0, 25871, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6,ipv6_src={{.IP6LOCAL}}"), "normal"),
						F(0, 24771, T("dl_dst={{.MAC}},ipv6,ipv6_dst={{.IP6LOCAL}}"), "normal"),
						// drop others
						F(0, 26860, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ipv6"), "drop"),
						F(0, 25860, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6"), "drop"),
						F(0, 24760, T("dl_dst={{.MAC}},ipv6"), "drop"),
					)
				}
			}
			// flows = append(flows,
			//	F(0, 23600, T("in_port={{.PortNo}},dl_src={{.MAC}}"), "normal"),
			// )
		}
		flows = append(flows,
			F(0, 25760, T("in_port={{.PortNo}},arp"), "drop"),
			F(0, 24660, T("in_port={{.PortNo}}"), "drop"),
		)
	}
	if !g.HostConfig.DisableSecurityGroup {
		secRules := g.GetNicSecurityRules(nic)

		flows = append(flows, secRules.Flows(g, nic, m)...)
	}
	return flows, nil
}

func (g *Guest) FlowsMap() (map[string][]*ovs.Flow, error) {
	r := map[string][]*ovs.Flow{}
	allGood := true
	for _, nic := range g.NICs {
		flows, err := g.FlowsMapForNic(nic)
		if err != nil {
			log.Warningf("FlowsMapForNic %s fail: %s", nic.MAC, err)
			allGood = false
			continue
		}
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

func (g *Guest) eachIP(data map[string]interface{}, cb func(func(string) string)) {
	data2 := map[string]interface{}{}
	for k, v := range data {
		data2[k] = v
	}
	var ipAddrs = data2["SubIPs"].([]string)
	if ip, ok := data2["IP"]; ok {
		ipAddrs = append(ipAddrs, ip.(string))
	}
	for _, ipAddr := range ipAddrs {
		data2["IP"] = ipAddr
		T2 := t(data2)
		cb(T2)
	}
}

func (sr *SecurityRules) Flows(g *Guest, nic *GuestNIC, data map[string]interface{}) []*ovs.Flow {
	if len(nic.IP) > 0 {
		data["IP"] = nic.IP
	} else {
		delete(data, "IP")
	}
	if len(nic.IP6) > 0 {
		data["IP6"] = nic.IP6
	} else {
		delete(data, "IP6")
	}
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
		F(0, 27300, T("in_port=LOCAL,dl_dst={{.MAC}},ip"), loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
		F(0, 27300, T("in_port=LOCAL,dl_dst={{.MAC}},ipv6"), loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
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
		flows = append(flows,
			F(0, 26870, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ipv6"),
				loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
			F(0, 25870, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6"),
				loadReg0BitVm+","+loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
			F(0, 24770, T("dl_dst={{.MAC}},ipv6"),
				loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
		)
	} else {
		if nic.EnableIPv4() {
			g.eachIP(data, func(T2 func(string) string) {
				flows = append(flows,
					F(0, 26870, T2("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ip,nw_dst={{.IP}}"),
						loadZone+T2(",ct(table=1,zone={{.CT_ZONE}})")),
					F(0, 25870, T2("in_port={{.PortNo}},dl_src={{.MAC}},ip,nw_src={{.IP}}"),
						loadReg0BitVm+","+loadZone+T2(",ct(table=1,zone={{.CT_ZONE}})")),
					F(0, 24770, T2("dl_dst={{.MAC}},ip,nw_dst={{.IP}}"),
						loadZone+T2(",ct(table=1,zone={{.CT_ZONE}})")),
				)
			})
			flows = append(flows,
				F(0, 26860, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ip"), "drop"),
				F(0, 25860, T("in_port={{.PortNo}},dl_src={{.MAC}},ip"), "drop"),
				F(0, 24760, T("dl_dst={{.MAC}},ip"), "drop"),
			)
		}

		if len(nic.IP6) > 0 {
			flows = append(flows,
				F(0, 26870, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ipv6,ipv6_dst={{.IP6}}"),
					loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
				F(0, 25870, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6,ipv6_src={{.IP6}}"),
					loadReg0BitVm+","+loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
				F(0, 24770, T("dl_dst={{.MAC}},ipv6,ipv6_dst={{.IP6}}"),
					loadZone+T(",ct(table=1,zone={{.CT_ZONE}})")),
			)
			flows = append(flows,
				F(0, 26860, T("in_port={{.PortNoPhy}},dl_dst={{.MAC}},{{._dl_vlan}},ipv6"), "drop"),
				F(0, 25860, T("in_port={{.PortNo}},dl_src={{.MAC}},ipv6"), "drop"),
				F(0, 24760, T("dl_dst={{.MAC}},ipv6"), "drop"),
			)
		}
	}
	flows = append(flows,
		F(0, 25600, T("in_port={{.PortNo}},dl_src={{.MAC}}"), "normal"),
	)

	if !g.HostConfig.SdnAllowConntrackInvalid {
		// ct_state= flags order matters
		flows = append(flows,
			F(1, 7900, "ip,ct_state=+inv+trk", "drop"),
			F(1, 7900, "ipv6,ct_state=+inv+trk", "drop"),
		)
	} else {
		flows = append(flows,
			F(1, 7650, T("ip,ct_state=+inv+trk,{{._in_port_not_vm}}"), "resubmit(,3)"),
			F(1, 7650, T("ipv6,ct_state=+inv+trk,{{._in_port_not_vm}}"), "resubmit(,3)"),
			F(1, 7640, T("ip,ct_state=+inv+trk,{{._in_port_vm}}"), "resubmit(,2)"),
			F(1, 7640, T("ipv6,ct_state=+inv+trk,{{._in_port_vm}}"), "resubmit(,2)"),
		)
	}
	flows = append(flows,
		F(1, 7800, T("ip,ct_state=+new+trk,{{._in_port_not_vm}}"), "resubmit(,3)"),
		F(1, 7800, T("ipv6,ct_state=+new+trk,{{._in_port_not_vm}}"), "resubmit(,3)"),
		F(1, 7700, T("ip,ct_state=+new+trk,{{._in_port_vm}}"), "resubmit(,2)"),
		F(1, 7700, T("ipv6,ct_state=+new+trk,{{._in_port_vm}}"), "resubmit(,2)"),
		F(1, 7600, "ip", "resubmit(,4)"),
		F(1, 7600, "ipv6", "resubmit(,4)"),
		F(4, 5600, T("ip,dl_dst={{.MAC}}"), loadZoneDstVM+",resubmit(,5),"),
		F(4, 5600, T("ipv6,dl_dst={{.MAC}}"), loadZoneDstVM+",resubmit(,5),"),
		F(4, 5500, "ip", "ct(commit,zone=NXM_NX_REG0[0..15]),normal"),
		F(4, 5500, "ipv6", "ct(commit,zone=NXM_NX_REG0[0..15]),normal"),
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
	flows = append(flows, F(3, 30, "ipv6", "ct(commit,zone=NXM_NX_REG0[0..15]),normal"))

	flows = append(flows,
		F(5, 20, T("ip,{{._in_port_not_vm}}"), "ct(commit,zone=NXM_NX_REG1[0..15]),normal"),
		F(5, 20, T("ipv6,{{._in_port_not_vm}}"), "ct(commit,zone=NXM_NX_REG1[0..15]),normal"),
		F(5, 10, T("ip,{{._in_port_vm}}"), "ct(commit,zone=NXM_NX_REG1[0..15]),ct(commit,zone=NXM_NX_REG0[0..15]),normal"),
		F(5, 10, T("ipv6,{{._in_port_vm}}"), "ct(commit,zone=NXM_NX_REG1[0..15]),ct(commit,zone=NXM_NX_REG0[0..15]),normal"),
	)
	return flows
}

// NOTE: KEEP THIS IN SYNC WITH CODE ABOVE
//
//	grep -oE '\<F\([0-9].*' pkg/agent/utils/flowsource.go  | sort -k 1.3,1.4n -k2r
//
// Assumptions
//
//  - MAC is unique, this can be an issue when !SrcMacCheck
//  - We try to not depend on IP uniqueness, but this is also requirement for LOCAL-vm communication
//  - We are the only user of ct_zone other than 0
//
// Flows with "VM" in them are guest nic-specific flows
//
// Table 0
// 40000 ipv6,actions=drop
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
// 25860 in_port=PORT_VM,dl_src=MAC_VM,ip,actions=drop                                  		SrcIpCheck
// 25770 in_port=PORT_VM,arp,dl_src=MAC_VM,arp_sha=MAC_VM,{SrcIpCheck?arp_spa=IP_VM},actions=normal	SrcMacCheck
// 25760 in_port=PORT_VM,arp,actions=drop                                               		SrcMacCheck
// 25600 in_port=PORT_VM,dl_src=MAC_VM,actions=normal
//
// 24770 dl_dst=MAC_VM,ip,{SrcIpCheck?nw_dst=IP_VM},actions=load_dst_VM_ZONE,ct(zone=dst_VM_ZONE,table=sec_CT)
// 24760 dl_dst=MAC_VM,ip,actions=drop                                              	SrcIpCheck
// 24670 in_port=PORT_VM,{!SrcMacCheck},actions=normal
// 24660 in_port=PORT_VM,{ SrcMacCheck},actions=drop
//
// 23700 in_port=PORT_PHY,{!SrcMacCheck},actions=normal
// 23600 in_port=PORT_PHY,{ SrcMacCheck},dl_dst=01:00:00:00:00:00/01:00:00:00:00:00,actions=normal
// 23500 in_port=PORT_PHY,{ SrcMacCheck},actions=drop
//
// Table 1 sec_CT
//  7900 ip,ct_state=+trk+inv,actions=drop						!allowInvalid
//  7800 ip,ct_state=+trk+new,{{!in_port_vm}},actions=resubmit(,sec_IN)
//  7700 ip,ct_state=+trk+new,{{ in_port_vm}},actions=resubmit(,sec_OUT)
//  7650 ip,ct_state=+trk+inv,{{!in_port_vm}},actions=resubmit(,sec_IN)			 allowInvalid
//  7640 ip,ct_state=+trk+inv,{{ in_port_vm}},actions=resubmit(,sec_OUT)		 allowInvalid
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
