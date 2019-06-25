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
				DHCPServerPort: 67,
			},
		},
		{
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
				DHCPServerPort: 1067,
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
