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
		data string
		want *HostConfig
	}{
		{
			data: "",
			want: &HostConfig{
				Port:           0,
				ServersPath:    "/opt/cloud/workspace/servers",
				K8sClusterCidr: defaultK8sCidr,
			},
		},
		{
			data: `
port = 8885
servers_path = '/opt/cloud/workspace/servers_owl'
networks = ['eth0/br0/10.168.222.136']
k8s_cluster_cidr = '10.44.0.0/17'
allow_switch_vms = True
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
			},
		},
	}
	for _, c := range cases {
		hc, err := newHostConfigFromBytes([]byte(c.data))
		if err != nil {
			t.Errorf("loading config failed: %v\n%s", err, c.data)
			continue
		}
		if !reflect.DeepEqual(hc, c.want) {
			t.Errorf("\ngot config\n  %#v\nwant\n  %#v", hc, c.want)
		}
	}
}
