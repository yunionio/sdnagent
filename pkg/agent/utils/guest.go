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
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path"
	"strings"

	"yunion.io/x/jsonutils"
	"yunion.io/x/pkg/errors"
	"yunion.io/x/pkg/util/netutils"
	"yunion.io/x/pkg/util/regutils"

	computeapi "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/hostman/guestman/desc"
	"yunion.io/x/onecloud/pkg/util/netutils2"
)

type guestDesc struct {
	NICs               []*GuestNIC          `json:"nics"`
	SecurityRules      string               `json:"security_rules"`
	AdminSecurityRules string               `json:"admin_security_rules"`
	NicSecgroups       []*GuestNICSecgroups `json:"nic_secgroups"`
	Name               string

	IsMaster       bool   `json:"is_master"`
	IsSlave        bool   `json:"is_slave"`
	HostId         string `json:"host_id"`
	IsVolatileHost bool   `json:"is_volatile_host"`

	SrcIpCheck  bool `json:"src_ip_check"`
	SrcMacCheck bool `json:"src_mac_check"`
}

func newGuestDesc() *guestDesc {
	desc := &guestDesc{
		IsMaster:    false,
		IsSlave:     false,
		SrcIpCheck:  true,
		SrcMacCheck: true,
	}
	return desc
}

type GuestNICSecgroups struct {
	SecurityRules string `json:"security_rules"`
	Mac           string `json:"mac"`
	Index         int    `json:"index"`
}

type GuestNIC struct {
	Bridge     string
	Bw         int
	Dns        string
	Domain     string
	Driver     string
	Gateway    string
	IfnameHost string `json:"ifname"`
	Index      int
	IfnameVM   string   `json:"interface"`
	IP         string   `json:"ip"`
	VirtualIps []string `json:"virtual_ips"`
	MAC        string
	Masklen    int
	Net        string
	NetId      string `json:"net_id"`
	Virtual    bool
	VLAN       int
	WireId     string      `json:"wire_id"`
	HostId     string      `json:"host_id"`
	Vpc        GuestNICVpc `json:"vpc"`

	IP6      string `json:"ip6"`
	Gateway6 string `json:"gateway6"`
	Masklen6 int    `json:"masklen6"`

	CtZoneId    uint16 `json:"-"`
	CtZoneIdSet bool   `json:"-"`
	PortNo      int    `json:"-"`

	SecurityRules *SecurityRules `json:"-"`

	NetworkAddresses []GuestNICNetworkAddress `json:"networkaddresses"`

	PortMappings computeapi.GuestPortMappings `json:"port_mappings"`
}

func (nic *GuestNIC) EnableIPv4() bool {
	return len(nic.IP) > 0
}

func (nic *GuestNIC) EnableIPv6() bool {
	return len(nic.IP6) > 0
}

type GuestNICNetworkAddress struct {
	Type    string `json:"type"`
	IpAddr  string `json:"ip_addr"`
	Masklen int    `json:"masklen"`
	Gateway string `json:"gateway"`
}

type GuestNICVpc struct {
	Id           string
	Provider     string `json:"provider"`
	MappedIpAddr string `json:"mapped_ip_addr"`

	MappedIp6Addr string `json:"mapped_ip6_addr"`
}

func (n *GuestNIC) TcData() *TcData {
	return &TcData{
		Type:        TC_DATA_TYPE_GUEST,
		Ifname:      n.IfnameHost,
		IngressMbps: uint64(n.Bw),
	}
}

func (n *GuestNIC) Map() map[string]interface{} {
	m := map[string]interface{}{
		// "IP":      n.IP,
		"SubIPs":  n.SubIPs(),
		"MAC":     n.MAC,
		"VLAN":    n.VLAN & 0xfff,
		"CT_ZONE": n.CtZoneId,
		"PortNo":  n.PortNo,
	}
	if len(n.IP) > 0 {
		m["IP"] = n.IP
	}
	if len(n.IP6) > 0 {
		m["IP6"] = n.IP6
		linkLocal, _ := netutils.Mac2LinkLocal(n.MAC)
		m["IP6LOCAL"] = linkLocal.String() // "fe80::/64"

		ip6 := net.ParseIP(n.IP6)
		m["IP6McastIP"] = netutils2.IP2SolicitMcastIP(ip6).String()
		m["IP6McastMac"] = netutils2.IP2SolicitMcastMac(ip6).String()
	}
	vlanTci := n.VLAN & 0xfff
	if n.VLAN > 1 {
		// 802.1Q vlan header present
		vlanTci |= 0x1000
	} else {
		vlanTci = 0
	}
	m["VLANTci"] = fmt.Sprintf("0x%04x/0x1fff", vlanTci)
	return m
}

func (n *GuestNIC) SubIPs() []string {
	var (
		ipAddrs []string
		nas     = n.NetworkAddresses
	)
	for i := range nas {
		na := &nas[i]
		if na.Type == "sub_ip" {
			ipAddrs = append(ipAddrs, na.IpAddr)
		}
	}
	ipAddrs = append(ipAddrs, n.VirtualIps...)
	return ipAddrs
}

type Guest struct {
	Id         string
	Path       string
	HostConfig *HostConfig

	Name          string
	SecurityRules *SecurityRules
	NICs          []*GuestNIC
	VpcNICs       []*GuestNIC
	HostId        string

	srcIpCheck  bool
	srcMacCheck bool

	isSlave        bool
	isVolatileHost bool
}

func (g *Guest) IsVM() bool {
	startvmPath := path.Join(g.Path, "startvm")
	_, err := os.Stat(startvmPath)
	if err == nil {
		return true
	}
	return false
}

func (g *Guest) Running() bool {
	pidPath := path.Join(g.Path, "pid")
	pidData, err := os.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		// NOTE just to be reservative
		return true
	}
	if len(pidData) == 0 {
		return false
	}
	// NOTE check /proc/<pid>/exe links a qemu executable
	pidStr := strings.TrimSpace(string(pidData))
	procPath := path.Join("/proc", pidStr)
	fileInfo, err := os.Stat(procPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		// NOTE just to be reservative
		return true
	}
	if fileInfo.IsDir() {
		return true
	}
	return false
}

func (g *Guest) IsVolatileHost() bool {
	return g.isVolatileHost || g.isSlave
}

func (g *Guest) GetJSONObjectDesc() (*desc.SGuestDesc, error) {
	descPath := path.Join(g.Path, "desc")
	data, err := os.ReadFile(descPath)
	if err != nil {
		return nil, errors.Wrap(err, "ReadFile")
	}
	obj, err := jsonutils.Parse(data)
	if err != nil {
		return nil, errors.Wrap(err, "json.Parse")
	}
	desc := desc.SGuestDesc{}
	err = obj.Unmarshal(&desc)
	if err != nil {
		return nil, errors.Wrap(err, "Unmarshal")
	}
	return &desc, nil
}

func (g *Guest) LoadDesc() error {
	descPath := path.Join(g.Path, "desc")
	descFile, err := os.Open(descPath)
	if err != nil {
		return err
	}
	defer descFile.Close()
	dec := json.NewDecoder(descFile)
	desc := newGuestDesc()
	err = dec.Decode(&desc)
	if err != nil {
		return err
	}
	g.Name = desc.Name
	g.HostId = desc.HostId
	g.NICs = desc.NICs

	g.VpcNICs = nil

	{
		rstr := desc.AdminSecurityRules + "; " + desc.SecurityRules
		rs, err := NewSecurityRules(rstr)
		if err != nil {
			return err
		}
		g.SecurityRules = rs
	}

	for i := len(g.NICs) - 1; i >= 0; i-- {
		nic := g.NICs[i]
		if rstr, ok := g.nicHasDedicatedSecgroups(nic.MAC, desc.NicSecgroups); ok {
			rs, err := NewSecurityRules(rstr)
			if err != nil {
				return errors.Wrapf(err, "nic %s NewSecurityRules %s failed", nic.MAC, rstr)
			}
			nic.SecurityRules = rs
		}

		if nic.Vpc.Provider != "" {
			g.VpcNICs = append(g.VpcNICs, nic)
			g.NICs = append(g.NICs[:i], g.NICs[i+1:]...)
		}
	}
	g.isVolatileHost = desc.IsVolatileHost
	if !desc.IsMaster && desc.IsSlave {
		g.isSlave = true
	} else {
		g.isSlave = false
	}

	g.srcIpCheck = desc.SrcIpCheck
	g.srcMacCheck = desc.SrcMacCheck
	if !g.srcMacCheck && g.srcIpCheck {
		g.srcIpCheck = false
	}
	return nil
}

func (g *Guest) nicHasDedicatedSecgroups(mac string, guestNicSecgroups []*GuestNICSecgroups) (string, bool) {
	for i := range guestNicSecgroups {
		if guestNicSecgroups[i].Mac == mac {
			return guestNicSecgroups[i].SecurityRules, true
		}
	}
	return "", false
}

func (g *Guest) GetNicSecurityRules(nic *GuestNIC) *SecurityRules {
	if nic.SecurityRules != nil {
		return nic.SecurityRules
	}
	return g.SecurityRules
}

func (g *Guest) NeedsSync() bool {
	if g.HostId == "" {
		if len(g.VpcNICs) > 0 {
			return true
		}
	}
	return false
}

func (g *Guest) SrcIpCheck() bool {
	if !g.HostConfig.AllowRouterVMs {
		return true
	}
	return g.srcIpCheck
}

func (g *Guest) SrcMacCheck() bool {
	if !g.HostConfig.AllowSwitchVMs {
		return true
	}
	return g.srcMacCheck
}

func (g *Guest) FindNicByNetIdIP(netId, ip string) *GuestNIC {
	var searchNic = func(nics []*GuestNIC) *GuestNIC {
		for _, nic := range nics {
			if nic.NetId == netId && nic.IP == ip {
				return nic
			}
		}
		return nil
	}
	if nic := searchNic(g.VpcNICs); nic != nil {
		return nic
	}
	if nic := searchNic(g.NICs); nic != nil {
		return nic
	}
	return nil
}

func (g *Guest) FindNicByHostLocalIP(hostLocal *HostLocal, ip string) *GuestNIC {
	var searchNic = func(nics []*GuestNIC) *GuestNIC {
		for _, nic := range nics {
			if nic.Bridge == hostLocal.Bridge {
				if regutils.MatchIP4Addr(ip) {
					_, fakeIp, _ := hostLocal.fakeMdSrcIpMac(nic.PortNo)
					if fakeIp == ip {
						return nic
					}
				} else {
					for _, metaSrvIp6 := range g.HostConfig.MetadataServerIp6s {
						_, fakeIp, _ := hostLocal.fakeMdSrcIp6Mac(nic.PortNo, metaSrvIp6)
						if fakeIp == ip {
							return nic
						}
					}
				}
			}
		}
		return nil
	}
	if nic := searchNic(g.NICs); nic != nil {
		return nic
	}
	return nil
}
