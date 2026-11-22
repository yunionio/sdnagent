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
	"fmt"
	"strings"

	"github.com/coreos/go-iptables/iptables"

	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"

	compute "yunion.io/x/onecloud/pkg/apis/compute"
)

const (
	iptablesNatTable             = "nat"
	iptablesPortMapCommentPrefix = "sdnagent-pm-"
)

var iptablesPortMapChains = []string{"PREROUTING", "OUTPUT"}

func portMapComment(guestId string) string {
	return iptablesPortMapCommentPrefix + guestId
}

// SyncGuestPortMappingDNAT syncs iptables DNAT rules for guest port mappings
// into nat table PREROUTING and OUTPUT chains.
// Match: host_ip:host_port -> DNAT to guest_ip:port.
func SyncGuestPortMappingDNAT(guestId string, nics []*GuestNIC) error {
	ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
	if err != nil {
		return errors.Wrap(err, "iptables client")
	}
	if err := ensureNatPortMapChains(ipt); err != nil {
		return errors.Wrap(err, "ensureNatPortMapChains")
	}

	comment := portMapComment(guestId)
	if err := clearPortMappingDNATByComment(ipt, comment); err != nil {
		return errors.Wrap(err, "clearPortMappingDNATByComment")
	}

	for _, nic := range nics {
		if !nic.EnableIPv4() || len(nic.PortMappings) == 0 {
			continue
		}
		for _, pm := range nic.PortMappings {
			if err := ensurePortMappingDNAT(ipt, comment, nic.IP, pm); err != nil {
				return errors.Wrap(err, "ensurePortMappingDNAT")
			}
		}
	}
	return nil
}

// ClearGuestPortMappingDNAT removes iptables DNAT rules previously installed
// for the given guest.
func ClearGuestPortMappingDNAT(guestId string) error {
	ipt, err := iptables.NewWithProtocol(iptables.ProtocolIPv4)
	if err != nil {
		return errors.Wrap(err, "iptables client")
	}
	return clearPortMappingDNATByComment(ipt, portMapComment(guestId))
}

func ensureNatPortMapChains(ipt *iptables.IPTables) error {
	for _, chain := range iptablesPortMapChains {
		exists, err := ipt.ChainExists(iptablesNatTable, chain)
		if err != nil {
			return errors.Wrapf(err, "check nat chain %s", chain)
		}
		if !exists {
			return errors.Errorf("nat chain %s does not exist", chain)
		}
	}
	return nil
}

func ensurePortMappingDNAT(ipt *iptables.IPTables, comment, guestIP string, pm *compute.GuestPortMapping) error {
	if pm == nil {
		return nil
	}
	if pm.HostPort == nil {
		return nil
	}
	if guestIP == "" || pm.Port <= 0 {
		return nil
	}

	proto := string(pm.Protocol)
	if proto == "" {
		proto = string(compute.GuestPortMappingProtocolTCP)
	}

	remoteIps := pm.RemoteIps
	if len(remoteIps) == 0 {
		remoteIps = []string{""}
	}

	for _, remoteIp := range remoteIps {
		if remoteIp == "0.0.0.0/0" {
			remoteIp = ""
		}
		spec := make([]string, 0, 16)
		if remoteIp != "" {
			spec = append(spec, "-s", remoteIp)
		}
		spec = append(spec,
			"-p", proto,
			"-m", proto,
			"--dport", fmt.Sprintf("%d", *pm.HostPort),
			"-m", "comment", "--comment", comment,
			"-j", "DNAT",
			"--to-destination", fmt.Sprintf("%s:%d", guestIP, pm.Port),
		)
		for _, chain := range iptablesPortMapChains {
			if err := ipt.AppendUnique(iptablesNatTable, chain, spec...); err != nil {
				return errors.Wrapf(err, "append DNAT %s -> %s:%d to nat/%s",
					pm.HostIp, guestIP, pm.Port, chain)
			}
		}
	}
	return nil
}

func clearPortMappingDNATByComment(ipt *iptables.IPTables, comment string) error {
	for _, chain := range iptablesPortMapChains {
		exists, err := ipt.ChainExists(iptablesNatTable, chain)
		if err != nil {
			return errors.Wrapf(err, "check nat chain %s", chain)
		}
		if !exists {
			continue
		}
		rules, err := ipt.List(iptablesNatTable, chain)
		if err != nil {
			return errors.Wrapf(err, "list nat/%s", chain)
		}
		for _, rule := range rules {
			if !strings.Contains(rule, comment) {
				continue
			}
			fields := strings.Fields(rule)
			// iptables -S: "-A CHAIN <rulespec...>"
			if len(fields) < 3 || fields[0] != "-A" {
				continue
			}
			if err := ipt.Delete(iptablesNatTable, chain, fields[2:]...); err != nil {
				log.Warningf("delete portmap DNAT from nat/%s: %s: %v", chain, rule, err)
			}
		}
	}
	return nil
}
