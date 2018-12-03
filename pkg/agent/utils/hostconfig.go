package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os/exec"
	"strings"
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
}

var snippet_pre = []byte(`
from __future__ import print_function
port = None
listen_interface = None
networks = []
servers_path = "/opt/cloud/workspace/servers"
k8s_cluster_cidr = '10.43.0.0/16'
allow_switch_vms = False

`)
var snippet_post = []byte(`

import json
print(json.dumps({
	'port': port,
	'networks': networks,
	'listen_interface': listen_interface,
	'servers_path': servers_path,
	'k8s_cluster_cidr': k8s_cluster_cidr,
	'allow_switch_vms': bool(allow_switch_vms),
}))
`)

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
	config, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	// NOTE python hack
	config = append(snippet_pre, config...)
	config = append(config, snippet_post...)
	cmd := exec.Command("python", "-c", string(config))
	jstr, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	// parse json dump
	v := struct {
		Port           int
		Networks       []string
		ServersPath    string `json:"servers_path"`
		K8sClusterCidr string `json:"k8s_cluster_cidr"`
		AllowSwitchVMs bool   `json:"allow_switch_vms"`
	}{}
	err = json.Unmarshal(jstr, &v)
	if err != nil {
		return nil, err
	}

	hc := &HostConfig{
		Port:           v.Port,
		ServersPath:    v.ServersPath,
		AllowSwitchVMs: v.AllowSwitchVMs,
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
