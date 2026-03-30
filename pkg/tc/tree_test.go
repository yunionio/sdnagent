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
	"reflect"
	"testing"

	"yunion.io/x/jsonutils"
)

func TestQdiscTree(t *testing.T) {
	cases := []struct {
		qdisc         string
		class         string
		filter        string
		wantQdiscTree *QdiscTree
		deltaLines    []string
	}{
		{
			qdisc: `qdisc htb 1: root refcnt 2 r2q 10 default 0x2 direct_packets_stat 6 direct_qlen 1000
qdisc clsact ffff: parent ffff:fff1 `,
			class: `class htb 1:1 root rate 10Gbit ceil 10Gbit burst 0b cburst 0b 
class htb 1:2 parent 1:1 prio 0 rate 1Gbit ceil 10Gbit burst 1375b cburst 0b 
class htb 1:3 parent 1:1 prio 0 rate 100Mbit ceil 100Mbit burst 1600b cburst 1600b`,
			filter: `filter parent 1: protocol ip pref 1 fw chain 0 
filter parent 1: protocol ip pref 1 fw chain 0 handle 0x257 classid 1:3`,
			wantQdiscTree: func() *QdiscTree {
				qdisc := &QdiscHtb{
					SBaseTcQdisc: &SBaseTcQdisc{
						Kind:   "htb",
						Handle: "1:",
						Parent: "",
					},
					DefaultClass: 0x2,
				}
				rootHtbClass := &SHtbClass{
					SBaseTcClass: &SBaseTcClass{
						Kind:    "htb",
						ClassId: "1:1",
						Parent:  nil,
					},
					Rate: 10000000000,
					Ceil: 10000000000,
				}
				return NewQdiscTree([]IQdisc{qdisc}, []IClass{
					rootHtbClass,
					&SHtbClass{
						SBaseTcClass: &SBaseTcClass{
							Kind:    "htb",
							ClassId: "1:2",
							Parent:  rootHtbClass,
						},
						Rate: 1000000000,
						Ceil: 10000000000,
					},
					&SHtbClass{
						SBaseTcClass: &SBaseTcClass{
							Kind:    "htb",
							ClassId: "1:3",
							Parent:  rootHtbClass,
						},
						Rate: 100000000,
						Ceil: 100000000,
					},
				}, []IFilter{
					&SFwFilter{
						SBaseTcFilter: &SBaseTcFilter{
							Kind:     "fw",
							Prio:     1,
							Protocol: "ip",
							Parent:   qdisc,
						},
						ClassId: "1:3",
						Handle:  0x257,
					},
				})
			}(),
			deltaLines: []string{},
		},
		{
			qdisc: `qdisc htb 1: root refcnt 2 r2q 10 default 0x2 direct_packets_stat 6 direct_qlen 1000
qdisc clsact ffff: parent ffff:fff1 `,
			class: `class htb 1:1 root rate 10Gbit ceil 10Gbit burst 0b cburst 0b 
class htb 1:2 parent 1:1 prio 0 rate 1Gbit ceil 10Gbit burst 1375b cburst 0b 
class htb 1:3 parent 1:1 prio 0 rate 200Mbit ceil 200Mbit burst 1600b cburst 1600b`,
			filter: `filter parent 1: protocol ip pref 1 fw chain 0 
filter parent 1: protocol ip pref 1 fw chain 0 handle 0x257 classid 1:3`,
			wantQdiscTree: func() *QdiscTree {
				qdisc := &QdiscHtb{
					SBaseTcQdisc: &SBaseTcQdisc{
						Kind:   "htb",
						Handle: "1:",
						Parent: "",
					},
					DefaultClass: 0x2,
				}
				rootHtbClass := &SHtbClass{
					SBaseTcClass: &SBaseTcClass{
						Kind:    "htb",
						ClassId: "1:1",
						Parent:  nil,
					},
					Rate: 10000000000,
					Ceil: 10000000000,
				}
				return NewQdiscTree([]IQdisc{qdisc}, []IClass{
					rootHtbClass,
					&SHtbClass{
						SBaseTcClass: &SBaseTcClass{
							Kind:    "htb",
							ClassId: "1:2",
							Parent:  rootHtbClass,
						},
						Rate: 1000000000,
						Ceil: 10000000000,
					},
					&SHtbClass{
						SBaseTcClass: &SBaseTcClass{
							Kind:    "htb",
							ClassId: "1:3",
							Parent:  rootHtbClass,
						},
						Rate: 100000000,
						Ceil: 100000000,
					},
				}, []IFilter{
					&SFwFilter{
						SBaseTcFilter: &SBaseTcFilter{
							Kind:     "fw",
							Prio:     1,
							Protocol: "ip",
							Parent:   qdisc,
						},
						ClassId: "1:3",
						Handle:  0x257,
					},
				})
			}(),
			deltaLines: []string{
				"class replace dev eth0 parent 1:1 classid 1:3 htb rate 100Mbit ceil 100Mbit",
			},
		},
		{
			qdisc:  ``,
			class:  ``,
			filter: ``,
			wantQdiscTree: func() *QdiscTree {
				qdisc := &QdiscHtb{
					SBaseTcQdisc: &SBaseTcQdisc{
						Kind:   "htb",
						Handle: "1:",
						Parent: "",
					},
					DefaultClass: 0x2,
				}
				rootHtbClass := &SHtbClass{
					SBaseTcClass: &SBaseTcClass{
						Kind:    "htb",
						ClassId: "1:1",
						Parent:  nil,
					},
					Rate: 10000000000,
					Ceil: 10000000000,
				}
				return NewQdiscTree([]IQdisc{qdisc}, []IClass{
					rootHtbClass,
					&SHtbClass{
						SBaseTcClass: &SBaseTcClass{
							Kind:    "htb",
							ClassId: "1:2",
							Parent:  rootHtbClass,
						},
						Rate: 1000000000,
						Ceil: 10000000000,
					},
					&SHtbClass{
						SBaseTcClass: &SBaseTcClass{
							Kind:    "htb",
							ClassId: "1:3",
							Parent:  rootHtbClass,
						},
						Rate: 100000000,
						Ceil: 100000000,
					},
				}, []IFilter{
					&SFwFilter{
						SBaseTcFilter: &SBaseTcFilter{
							Kind:     "fw",
							Prio:     1,
							Protocol: "ip",
							Parent:   qdisc,
						},
						ClassId: "1:3",
						Handle:  0x257,
					},
				})
			}(),
			deltaLines: []string{
				"qdisc add dev eth0 root handle 1: htb default 0x2",
				"class add dev eth0 parent 1: classid 1:1 htb rate 10Gbit ceil 10Gbit",
				"class add dev eth0 parent 1:1 classid 1:2 htb rate 1Gbit ceil 10Gbit",
				"class add dev eth0 parent 1:1 classid 1:3 htb rate 100Mbit ceil 100Mbit",
				"filter add dev eth0 parent 1: protocol ip prio 1 handle 0x257 fw classid 1:3",
			},
		},
		{
			qdisc: `qdisc tbf 1: root refcnt 2 rate 100Mbit burst 12500b lat 100ms 
qdisc fq_codel 10: parent 1: limit 10240p flows 1024 quantum 1514 target 5ms interval 100ms memory_limit 32Mb ecn drop_batch 64 `,
			class:  `class tbf 1:1 parent 1: leaf 10: `,
			filter: ``,
			wantQdiscTree: func() *QdiscTree {
				qdisc := &QdiscTbf{
					SBaseTcQdisc: &SBaseTcQdisc{
						Kind:   "tbf",
						Handle: "1:",
						Parent: "",
					},
					Rate:    100000000,
					Burst:   12500,
					Latency: 100000,
				}
				return NewQdiscTree([]IQdisc{qdisc}, []IClass{}, []IFilter{})
			}(),
			deltaLines: []string{},
		},
		{
			qdisc:  ``,
			class:  ``,
			filter: ``,
			wantQdiscTree: func() *QdiscTree {
				qdisc := &QdiscTbf{
					SBaseTcQdisc: &SBaseTcQdisc{
						Kind:   "tbf",
						Handle: "1:",
						Parent: "",
					},
					Rate:    100000000,
					Burst:   12500,
					Latency: 100000,
				}
				return NewQdiscTree([]IQdisc{qdisc}, []IClass{}, []IFilter{})
			}(),
			deltaLines: []string{
				"qdisc add dev eth0 root handle 1: tbf rate 100Mbit burst 12500b latency 100ms",
			},
		},
	}
	for i, c := range cases {
		qt, err := NewQdiscTreeFromString(c.qdisc, c.class, c.filter)
		if err != nil {
			t.Fatalf("create qdisc tree error: %s", err)
		}
		deltaLines := c.wantQdiscTree.Delta(qt, "eth0")
		if !reflect.DeepEqual(deltaLines, c.deltaLines) {
			t.Fatalf("[case %d] delta lines want %v, got %v", i, jsonutils.Marshal(c.deltaLines), jsonutils.Marshal(deltaLines))
		}
	}
}
