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

	"yunion.io/x/sdnagent/pkg/tc"
)

func TestTcDataQdiscTree(t *testing.T) {
	mustTree := func(s string) *tc.QdiscTree {
		qt, err := tc.NewQdiscTreeFromString(s)
		if err != nil {
			t.Fatalf("making tree error: %s\n%s", err, s)
		}
		return qt
	}
	cases := []struct {
		name string
		in   TcData
		out  *tc.QdiscTree
	}{
		{
			name: "hostlocal",
			in: TcData{
				Type:        TC_DATA_TYPE_HOSTLOCAL,
				Ifname:      "dummy0",
				IngressMbps: 999,
			},
			out: mustTree("qdisc fq_codel root handle 1:\n"),
		},
		{
			name: "0 ingress (no limit)",
			in: TcData{
				Type:        TC_DATA_TYPE_GUEST,
				Ifname:      "dummy0",
				IngressMbps: 0,
			},
			out: mustTree("qdisc fq_codel root handle 1:"),
		},
		{
			name: "1000Mbps ingress",
			in: TcData{
				Type:        TC_DATA_TYPE_GUEST,
				Ifname:      "dummy0",
				IngressMbps: 1000,
			},
			out: mustTree(
				"qdisc tbf root handle 1: rate 1Gbit burst 125000b latency 100ms\n" +
					"qdisc fq_codel parent 1: handle 10:\n"),
		},
		{
			name: "33Mbps ingress",
			in: TcData{
				Type:        TC_DATA_TYPE_GUEST,
				Ifname:      "dummy0",
				IngressMbps: 33,
			},
			out: mustTree(
				"qdisc tbf root handle 1: rate 33Mbit burst 4125b latency 100ms\n" +
					"qdisc fq_codel parent 1: handle 10:\n"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			qt, err := c.in.QdiscTree()
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if !qt.Equals(c.out) {
				t.Fatalf("want:\n%s\ngot:\n%s", c.out, qt)
			}
		})
	}
}
