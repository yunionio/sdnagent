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
	"io/ioutil"
	"net"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v2"
)

type HostConfigNetwork struct {
	Bridge string
	Ifname string
	IP     net.IP
	mac    net.HardwareAddr
}

func (hcn *HostConfigNetwork) IPMAC() (net.IP, net.HardwareAddr, error) {
	if hcn.mac == nil {
		iface, err := net.InterfaceByName(hcn.Bridge)
		if err != nil {
			return nil, nil, err
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, nil, err
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				ip := ipnet.IP.To4()
				if ip != nil {
					hcn.IP = ip
					break
				}
			}
		}
		hcn.mac = iface.HardwareAddr
	}
	if hcn.IP != nil && hcn.mac != nil {
		return hcn.IP, hcn.mac, nil
	}
	return nil, nil, fmt.Errorf("cannot find proper ip/mac")
}

type HostConfig struct {
	Port           int
	Networks       []*HostConfigNetwork
	ServersPath    string
	K8sClusterCidr *net.IPNet
	AllowSwitchVMs bool // allow virtual machines act as switches
	AllowRouterVMs bool // allow virtual machines act as routers
	DHCPServerPort int
}

func (hc *HostConfig) MetadataPort() int {
	return hc.Port + 1000
}

func NewHostConfigNetwork(network string) (*HostConfigNetwork, error) {
	chunks := strings.Split(network, "/")
	if len(chunks) >= 3 {
		// the 3rd field can be an ip address or platform network name.
		// net.ParseIP will return nil when it fails
		return &HostConfigNetwork{
			Ifname: chunks[0],
			Bridge: chunks[1],
			IP:     net.ParseIP(chunks[2]),
		}, nil
	}
	return nil, fmt.Errorf("invalid host.conf networks config: %q", network)
}

func NewHostConfig(file string) (*HostConfig, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	return newHostConfigFromBytes(data)
}

func unmarshalPythonConfig(data []byte, out interface{}) error {
	var (
		snippet_pre = []byte(`
from __future__ import print_function
port = None
listen_interface = None
networks = []
servers_path = "/opt/cloud/workspace/servers"
k8s_cluster_cidr = '10.43.0.0/16'
allow_switch_vms = False
allow_router_vms = True
dhcp_server_port = 67

`)
		snippet_post = []byte(`

import json
print(json.dumps({
	'port': port,
	'networks': networks,
	'listen_interface': listen_interface,
	'servers_path': servers_path,
	'k8s_cluster_cidr': k8s_cluster_cidr,
	'allow_switch_vms': bool(allow_switch_vms),
	'allow_router_vms': bool(allow_router_vms),
	'dhcp_server_port': dhcp_server_port,
}))
`)
	)

	// NOTE python hack
	data = append(snippet_pre, data...)
	data = append(data, snippet_post...)
	cmd := exec.Command("python", "-c", string(data))
	jstr, err := cmd.Output()
	if err != nil {
		return err
	}
	return json.Unmarshal(jstr, out)
}

func newHostConfigFromBytes(data []byte) (*HostConfig, error) {
	// parse json dump
	v := struct {
		Port           int
		Networks       []string
		ServersPath    string `json:"servers_path" yaml:"servers_path"`
		K8sClusterCidr string `json:"k8s_cluster_cidr" yaml:"k8s_cluster_cidr"`
		AllowSwitchVMs bool   `json:"allow_switch_vms" yaml:"allow_switch_vms"`
		AllowRouterVMs bool   `json:"allow_router_vms" yaml:"allow_router_vms"`
		DHCPServerPort int    `json:"dhcp_server_port" yaml:"dhcp_server_port"`
	}{
		ServersPath:    "/opt/cloud/workspace/servers",
		K8sClusterCidr: "10.43.0.0/16",
		DHCPServerPort: 67,
		AllowRouterVMs: true,
	}
	{
		type funcType func([]byte, interface{}) error
		var (
			funcs = []funcType{
				unmarshalPythonConfig,
				yaml.Unmarshal,
			}
			err error
		)
		for _, f := range funcs {
			err = f(data, &v)
			if err == nil {
				break
			}
		}
		if err != nil {
			return nil, err
		}
	}
	if v.AllowSwitchVMs && !v.AllowRouterVMs {
		v.AllowRouterVMs = true
	}

	hc := &HostConfig{
		Port:           v.Port,
		ServersPath:    v.ServersPath,
		AllowSwitchVMs: v.AllowSwitchVMs,
		AllowRouterVMs: v.AllowRouterVMs,
		DHCPServerPort: v.DHCPServerPort,
	}
	_, k8sCidr, err := net.ParseCIDR(v.K8sClusterCidr)
	if err == nil {
		// it's an optional argument
		hc.K8sClusterCidr = k8sCidr
	}
	for _, network := range v.Networks {
		hcn, err := NewHostConfigNetwork(network)
		if err != nil {
			// NOTE error ignored
			continue
		}
		hc.Networks = append(hc.Networks, hcn)
	}
	return hc, nil
}

func (hc *HostConfig) HostNetworkConfig(bridge string) *HostConfigNetwork {
	for _, hcn := range hc.Networks {
		if hcn.Bridge == bridge {
			return hcn
		}
	}
	return nil
}
