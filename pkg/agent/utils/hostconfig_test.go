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
	"net"
	"reflect"
	"testing"
)

const configData = `
port = None
listen_interface = None
networks = []
servers_path = "/opt/cloud/workspace/servers"
k8s_cluster_cidr = '10.43.0.0/16'
allow_switch_vms = True
`

func TestHostConfig(t *testing.T) {
	_, defaultK8sCidr, _ := net.ParseCIDR("10.43.0.0/16")
	_, nonDefaultK8sCidr, _ := net.ParseCIDR("10.44.0.0/17")
	cases := []struct {
		name string
		data string
		want *HostConfig
	}{
		{
			name: "default",
			data: "",
			want: &HostConfig{
				Port:           0,
				ServersPath:    "/opt/cloud/workspace/servers",
				K8sClusterCidr: defaultK8sCidr,
				DHCPServerPort: 67,
				AllowRouterVMs: true,
			},
		},
		{
			name: "!allow_switch_vms,allow_router_vms",
			data: `
allow_switch_vms = False
allow_router_vms = True
			`,
			want: &HostConfig{
				Port:           0,
				ServersPath:    "/opt/cloud/workspace/servers",
				K8sClusterCidr: defaultK8sCidr,
				DHCPServerPort: 67,
				AllowSwitchVMs: false,
				AllowRouterVMs: true,
			},
		},
		{
			name: "allow_switch_vms,!allow_router_vms (overridden)",
			data: `
allow_switch_vms = True
allow_router_vms = False
			`,
			want: &HostConfig{
				Port:           0,
				ServersPath:    "/opt/cloud/workspace/servers",
				K8sClusterCidr: defaultK8sCidr,
				DHCPServerPort: 67,
				AllowSwitchVMs: true,
				AllowRouterVMs: true,
			},
		},
		{
			name: "normal",
			data: `
port = 8885
servers_path = '/opt/cloud/workspace/servers_owl'
networks = ['eth0/br0/10.168.222.136']
k8s_cluster_cidr = '10.44.0.0/17'
allow_switch_vms = True
dhcp_server_port = 1067
			`,
			want: &HostConfig{
				Port: 8885,
				Networks: []*HostConfigNetwork{
					&HostConfigNetwork{
						Bridge: "br0",
						Ifname: "eth0",
						IP:     net.IPv4(10, 168, 222, 136),
					},
				},
				ServersPath:    "/opt/cloud/workspace/servers_owl",
				K8sClusterCidr: nonDefaultK8sCidr,
				AllowSwitchVMs: true,
				AllowRouterVMs: true,
				DHCPServerPort: 1067,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hc, err := newHostConfigFromBytes([]byte(c.data))
			if err != nil {
				t.Errorf("loading config failed: %v\n%s", err, c.data)
			}
			if !reflect.DeepEqual(hc, c.want) {
				t.Errorf("\ngot config\n  %#v\nwant\n  %#v", hc, c.want)
			}
		})
	}
}
