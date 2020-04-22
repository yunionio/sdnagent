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
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v2"

	"yunion.io/x/onecloud/pkg/mcclient/auth"
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
	file  string
	mtime time.Time

	AuthURL             string
	Region              string
	AdminDomain         string
	AdminProject        string
	AdminProjectDomain  string
	AdminUser           string
	AdminPassword       string
	SessionEndpointType string

	EnableSsl   bool
	SslCaCerts  string
	SslCertfile string
	SslKeyfile  string

	Port           int
	Networks       []*HostConfigNetwork
	ServersPath    string
	K8sClusterCidr *net.IPNet
	AllowSwitchVMs bool // allow virtual machines act as switches
	AllowRouterVMs bool // allow virtual machines act as routers
	DHCPServerPort int

	OvnIntegrationBridge string
	OvnMappedBridge      string
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
	fi, err := os.Stat(file)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	hc, err := newHostConfigFromBytes(data)
	if err != nil {
		return nil, err
	}

	hc.file = file
	hc.mtime = fi.ModTime()
	return hc, nil
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
allow_switch_vms = True
allow_router_vms = True
dhcp_server_port = 67

ovn_integration_bridge = 'brvpc'
ovn_mapped_bridge = 'brmapped'

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

	'ovn_integration_bridge': ovn_integration_bridge,
	'ovn_mapped_bridge': ovn_mapped_bridge,
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
		AuthURL             string `json:"auth_url" yaml:"auth_url"`
		Region              string `json:"region" yaml:"region"`
		AdminDomain         string `json:"admin_domain" yaml:"admin_domain"`
		AdminProject        string `json:"admin_project" yaml:"admin_project"`
		AdminProjectDomain  string `json:"admin_project_domain" yaml:"admin_project_domain"`
		AdminUser           string `json:"admin_user" yaml:"admin_user"`
		AdminPassword       string `json:"admin_password" yaml:"admin_password"`
		SessionEndpointType string `json:"session_endpoint_type" yaml:"session_endpoint_type"`

		EnableSsl   bool   `json:"enable_ssl" yaml:"enable_ssl"`
		SslCaCerts  string `json:"ssl_ca_certs" yaml:"ssl_ca_certs"`
		SslCertfile string `json:"ssl_certfile" yaml:"ssl_certfile"`
		SslKeyfile  string `json:"ssl_keyfile" yaml:"ssl_keyfile"`

		Port           int
		Networks       []string
		ServersPath    string `json:"servers_path" yaml:"servers_path"`
		K8sClusterCidr string `json:"k8s_cluster_cidr" yaml:"k8s_cluster_cidr"`
		AllowSwitchVMs bool   `json:"allow_switch_vms" yaml:"allow_switch_vms"`
		AllowRouterVMs bool   `json:"allow_router_vms" yaml:"allow_router_vms"`
		DHCPServerPort int    `json:"dhcp_server_port" yaml:"dhcp_server_port"`

		OvnIntegrationBridge string `json:"ovn_integration_bridge"`
		OvnMappedBridge      string `json:"ovn_mapped_bridge"`
	}{
		ServersPath:    "/opt/cloud/workspace/servers",
		K8sClusterCidr: "10.43.0.0/16",
		DHCPServerPort: 67,
		AllowSwitchVMs: true,
		AllowRouterVMs: true,

		OvnIntegrationBridge: "brvpc",
		OvnMappedBridge:      "brmapped",
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
		AuthURL:             v.AuthURL,
		Region:              v.Region,
		AdminDomain:         v.AdminDomain,
		AdminProject:        v.AdminProject,
		AdminProjectDomain:  v.AdminProjectDomain,
		AdminUser:           v.AdminUser,
		AdminPassword:       v.AdminPassword,
		SessionEndpointType: v.SessionEndpointType,

		EnableSsl:   v.EnableSsl,
		SslCaCerts:  v.SslCaCerts,
		SslCertfile: v.SslCertfile,
		SslKeyfile:  v.SslKeyfile,

		Port:           v.Port,
		ServersPath:    v.ServersPath,
		AllowSwitchVMs: v.AllowSwitchVMs,
		AllowRouterVMs: v.AllowRouterVMs,
		DHCPServerPort: v.DHCPServerPort,

		OvnIntegrationBridge: v.OvnIntegrationBridge,
		OvnMappedBridge:      v.OvnMappedBridge,
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

func (hc *HostConfig) WatchMtimeChange(ctx context.Context, cb func(time.Time)) {
	tick := time.NewTicker(13 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			fi, err := os.Stat(hc.file)
			if mtime := fi.ModTime(); err == nil && !mtime.Equal(hc.mtime) {
				cb(mtime)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (hc *HostConfig) Auth(ctx context.Context) error {
	a := auth.NewAuthInfo(
		hc.AuthURL,
		hc.AdminDomain,
		hc.AdminUser,
		hc.AdminPassword,
		hc.AdminProject,
		hc.AdminProjectDomain,
	)

	if t := hc.SessionEndpointType; t != "" {
		if t != auth.PublicEndpointType && t != auth.InternalEndpointType {
			return fmt.Errorf("Invalid session endpoint type %q", t)
		}
		auth.SetEndpointType(t)
	}

	var (
		debugClient = false
		insecure    = true
		certfile    = hc.SslCertfile
		keyfile     = hc.SslKeyfile
	)
	if !hc.EnableSsl {
		certfile = ""
		keyfile = ""
	}
	auth.Init(a, debugClient, insecure, certfile, keyfile)
	return nil
}
