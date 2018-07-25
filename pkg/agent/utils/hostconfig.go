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
}

type HostConfig struct {
	Port           int
	ListenIfname   string
	Networks       []*HostConfigNetwork
	ServersPath    string
	K8sClusterCidr *net.IPNet
	AllowSwitchVMs bool // allow virtual machines act as switches
}

var snippet_pre []byte = []byte(`
from __future__ import print_function
port = None
listen_interface = None
networks = []
servers_path = "/opt/cloud/workspace/servers"
k8s_cluster_cidr = '10.43.0.0/16'
allow_switch_vms = False

`)
var snippet_post []byte = []byte(`

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
		Port            int
		ListenInterface string `json:"listen_interface"`
		Networks        []string
		ServersPath     string `json:"servers_path"`
		K8sClusterCidr  string `json:"k8s_cluster_cidr"`
		AllowSwitchVMs  bool   `json:"allow_switch_vms"`
	}{}
	err = json.Unmarshal(jstr, &v)
	if err != nil {
		return nil, err
	}

	hc := &HostConfig{
		Port:           v.Port,
		ListenIfname:   v.ListenInterface,
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

func (hc *HostConfig) MasterIP() (net.IP, error) {
	if len(hc.ListenIfname) > 0 {
		iface, err := net.InterfaceByName(hc.ListenIfname)
		if err != nil {
			return nil, err
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				return v.IP, nil
			case *net.IPAddr:
				return v.IP, nil
			}
		}
	} else {
		for _, hcn := range hc.Networks {
			if hcn.IP != nil {
				return hcn.IP, nil
			}
		}
	}
	return nil, fmt.Errorf("cannot find master ip")
}

func (hc *HostConfig) MasterMAC() (net.HardwareAddr, error) {
	if len(hc.ListenIfname) > 0 {
		iface, err := net.InterfaceByName(hc.ListenIfname)
		if err != nil {
			return nil, err
		}
		return iface.HardwareAddr, nil
	} else {
		for _, hcn := range hc.Networks {
			iface, err := net.InterfaceByName(hcn.Bridge)
			if err != nil {
				continue
			}
			return iface.HardwareAddr, nil
		}
	}
	return nil, fmt.Errorf("cannot find master mac")
}

func (hc *HostConfig) HostNetworkConfig(bridge string) *HostConfigNetwork {
	for _, hcn := range hc.Networks {
		if hcn.Bridge == bridge {
			return hcn
		}
	}
	return nil
}
