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
	"testing"

	"yunion.io/x/pkg/util/netutils"
)

func TestFakeMdSrcIpMac(t *testing.T) {
	cases := []struct {
		ip   string
		mac  string
		port int
	}{
		{
			ip:   "169.254.254.1",
			mac:  "e4:43:4b:06:65:42",
			port: 0,
		},
		{
			ip:   "169.254.254.1",
			mac:  "e4:43:4b:06:65:42",
			port: 1,
		},
		{
			ip:   "169.254.254.1",
			mac:  "e4:43:4b:06:65:42",
			port: 2,
		},
		{
			ip:   "169.254.254.1",
			mac:  "e4:43:4b:06:65:42",
			port: 3,
		},
		{
			ip:   "10.127.100.2",
			mac:  "e4:43:4b:06:65:40",
			port: 0,
		},
		{
			ip:   "10.127.100.2",
			mac:  "e4:43:4b:06:65:40",
			port: 1,
		},
	}
	for _, c := range cases {
		prefix, fakeIp, fakeMac := fakeMdSrcIpMac(c.ip, c.mac, c.port)
		t.Logf("%s %s %s", prefix, fakeIp, fakeMac)
	}
}

func TestFakeMdSrcIp6Mac(t *testing.T) {
	cases := []struct {
		mac        string
		port       int
		metaSrvIp6 string
	}{
		{
			mac:        "e4:43:4b:06:65:42",
			port:       1,
			metaSrvIp6: "fd00:ec2::254",
		},
		{
			mac:        "e4:43:4b:06:65:42",
			port:       2,
			metaSrvIp6: "fd00:ec2::254",
		},
		{
			mac:        "e4:43:4b:06:65:42",
			port:       3,
			metaSrvIp6: "fd00:ec2::254",
		},
		{
			mac:        "e4:43:4b:06:65:42",
			port:       4,
			metaSrvIp6: "fd00:ec2::254",
		},
		{
			mac:        "e4:43:4b:06:65:42",
			port:       5,
			metaSrvIp6: "fd00:ec2::254",
		},
	}
	for _, c := range cases {
		linkLocalV6, err := netutils.Mac2LinkLocal(c.mac)
		if err != nil {
			t.Errorf("failed to convert mac %s to link local v6: %v", c.mac, err)
		} else {
			t.Logf("linkLocalV6: %s", linkLocalV6.String())
			prefix, fakeIp6, fakeMac := fakeMdSrcIp6Mac(linkLocalV6.String(), c.mac, c.port, c.metaSrvIp6)
			t.Logf("%s %s %s", prefix, fakeIp6, fakeMac)
		}
	}
}
