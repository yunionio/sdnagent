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

package tc

import (
	"testing"

	"yunion.io/x/jsonutils"
)

type tcCase struct {
	ifname      string
	line        string
	lineDelete  string
	lineReplace string
	isRoot      bool
	wantQdisc   IQdisc
}

func TestQdiscTbf(t *testing.T) {
	cases := []tcCase{
		{
			ifname:      "wp1-136",
			line:        "qdisc tbf 1: root refcnt 2 rate 500000Kbit burst 64000b/1 mpu 0b lat 100.0ms",
			lineDelete:  "qdisc delete dev wp1-136 root handle 1:",
			lineReplace: "qdisc add dev wp1-136 root handle 1: tbf rate 500Mbit burst 64Kb latency 100ms",
			isRoot:      true,
			wantQdisc: &QdiscTbf{
				SBaseTcQdisc: &SBaseTcQdisc{
					Kind:   "tbf",
					Handle: "1:",
					Parent: "",
					Root:   true,
				},
				Rate:    500000000,
				Burst:   64000,
				Latency: 100000,
			},
		},
		{
			ifname:      "wp1-136",
			line:        "qdisc tbf 1: root refcnt 2 rate 500000Kbit burst 64000b/4 mpu 0b lat 100.0ms",
			lineDelete:  "qdisc delete dev wp1-136 root handle 1:",
			lineReplace: "qdisc add dev wp1-136 root handle 1: tbf rate 500Mbit burst 64Kb latency 100ms",
			isRoot:      true,
			wantQdisc: &QdiscTbf{
				SBaseTcQdisc: &SBaseTcQdisc{
					Kind:   "tbf",
					Handle: "1:",
					Parent: "",
					Root:   true,
				},
				Rate:    500000000,
				Burst:   64000,
				Latency: 100000,
			},
		},
		{
			ifname:      "br0",
			line:        "qdisc htb 1: root refcnt 2 r2q 10 default 0x2 direct_packets_stat 6 direct_qlen 1000",
			lineDelete:  "qdisc delete dev br0 root handle 1:",
			lineReplace: "qdisc add dev br0 root handle 1: htb default 0x2",
			isRoot:      true,
			wantQdisc: &QdiscHtb{
				SBaseTcQdisc: &SBaseTcQdisc{
					Kind:   "htb",
					Handle: "1:",
					Parent: "",
					Root:   true,
				},
				DefaultClass: 2,
			},
		},
		{
			ifname:      "eth0",
			line:        "qdisc tbf 1: root refcnt 2 rate 100Mbit burst 12500b lat 100ms",
			lineDelete:  "qdisc delete dev eth0 root handle 1:",
			lineReplace: "qdisc add dev eth0 root handle 1: tbf rate 100Mbit burst 12500b latency 100ms",
			isRoot:      true,
			wantQdisc: &QdiscTbf{
				SBaseTcQdisc: &SBaseTcQdisc{
					Kind:   "tbf",
					Handle: "1:",
					Parent: "",
					Root:   true,
				},
				Rate:    100000000,
				Burst:   12500,
				Latency: 100000,
			},
		},
	}

	for _, c := range cases {
		qs, err := parseQdiscLines([]string{c.line})
		if err != nil {
			t.Errorf("parseQdiscLines: %s", err)
			continue
		}
		if !qs[0].Equals(c.wantQdisc) {
			t.Errorf("Qdisc want %v, got %v", jsonutils.Marshal(c.wantQdisc), jsonutils.Marshal(qs[0]))
		}
		if lineDelete := qs[0].DeleteLine(c.ifname); lineDelete != c.lineDelete {
			t.Errorf("delete line want: %s, got: %s", c.lineDelete, lineDelete)
			continue
		}
		if lineReplace := qs[0].AddLine(c.ifname); lineReplace != c.lineReplace {
			t.Errorf("add line want: %s, got: %s", c.lineReplace, lineReplace)
			continue
		}
	}
}
