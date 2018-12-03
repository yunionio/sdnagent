package utils

import "testing"

func TestHostConfig(t *testing.T) {
	hc, err := NewHostConfig("/etc/yunion/host.conf")
	if err != nil {
		t.Fatalf("hostconfig: load error: %s", err)
	}
	t.Logf("hostconfig: port: %d", hc.Port)
	t.Logf("hostconfig: servers_path: %s", hc.ServersPath)
	t.Logf("hostconfig: k8s_cluster_cidr: %s", hc.K8sClusterCidr)
	hcn := hc.HostNetworkConfig("br0")
	if hcn == nil {
		t.Fatalf("hostconfig: cannot find network config for %s", "br0")
	}
	t.Logf("hostconfig: %s/%s/%s", hcn.Ifname, hcn.Bridge, hcn.IP)
	for _, hcn := range hc.Networks {
		IP, MAC, err := hcn.IPMAC()
		if err != nil {
			t.Fatalf("hcn %s: %s", hcn.Bridge, err)
		}
		t.Logf("hcn %s: IP/MAC: %s/%s", hcn.Bridge, IP, MAC)
	}
}
