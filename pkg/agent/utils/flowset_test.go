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

	"github.com/digitalocean/go-openvswitch/ovs"
)

func TestFlowSet(t *testing.T) {
	flowStrs := []string{
		`priority=24770,ipv6,dl_dst=00:22:1b:04:95:b3,ipv6_dst=fc00:0:1:1004:ac1f:68f2:1b04:95b3,table=0,idle_timeout=0,actions=load:0xf2e5->NXM_NX_REG0[0..15],ct(table=1,zone=62181)`,
		`priority=24770,ipv6,dl_dst=00:22:68:f7:fe:5b,ipv6_dst=fc00:0:1:1004:3d45:44ef:ad1e:5ba5,table=0,idle_timeout=0,actions=load:0xeef6->NXM_NX_REG0[0..15],ct(table=1,zone=61174)`,
		`priority=24770,ipv6,dl_dst=00:22:da:ab:44:21,ipv6_dst=fc00:0:1:1004:bd7e:6319:2e92:3f1d,table=0,idle_timeout=0,actions=load:0xfe04->NXM_NX_REG0[0..15],ct(table=1,zone=65028)`,
		`priority=24770,ipv6,dl_dst=00:22:24:48:23:22,ipv6_dst=fc00:0:1:1004:ac1f:68f3:2448:2322,table=0,idle_timeout=0,actions=load:0xf524->NXM_NX_REG0[0..15],ct(table=1,zone=62756)`,
		`priority=24770,ipv6,dl_dst=00:22:14:33:ca:53,ipv6_dst=fc00:0:1:1004:ac1f:68f0:1433:ca53,table=0,idle_timeout=0,actions=load:0xf0f6->NXM_NX_REG0[0..15],ct(table=1,zone=61686)`,
		`priority=24770,ipv6,dl_dst=00:22:1c:ba:c0:9d,ipv6_dst=fc00:0:1:1004:ac1f:68f1:1cba:c09d,table=0,idle_timeout=0,actions=load:0xedbe->NXM_NX_REG0[0..15],ct(table=1,zone=60862)`,
		`priority=24770,ipv6,dl_dst=00:22:e6:6c:23:4e,ipv6_dst=fc00:0:1:1004:4a87:de4e:2fd8:6798,table=0,idle_timeout=0,actions=load:0xee4d->NXM_NX_REG0[0..15],ct(table=1,zone=61005)`,
	}
	fs0 := FlowSet{}
	for i := range flowStrs {
		fstr := flowStrs[i]
		flow := &ovs.Flow{}
		err := flow.UnmarshalText([]byte(fstr))
		if err != nil {
			t.Errorf("unmarshal %s fail %s", fstr, err)
		} else {
			fs0.Add(flow)
		}
	}
	fs0Str, err := fs0.DumpFlows()
	if err != nil {
		t.Errorf("fs0.DumpFlows error %s", err)
	} else {
		t.Logf("flows %d: %s", len(fs0.flows), fs0Str)
	}
}

func TestFlowMarshal(t *testing.T) {
	cases := []string{
		`cookie=0x0, duration=20000.123s, table=0, n_packets=0, n_bytes=0, idle_age=100, priority=20000,in_port=LOCAL,dl_dst=33:33:33:ff:e0:26,ip actions=load:0x89f1->NXM_NX_REG0[0..15],load:0x1->NXM_NX_REG0[16],ct(table=1,zone=62181)`,
		`cookie=0x0, duration=12828.711s, table=0, n_packets=0, n_bytes=0, idle_age=12828, priority=40012,icmp6,in_port=28,icmp_type=135,nd_target=fe80::a9fe:a9fe actions=learn(table=12,idle_timeout=30,priority=20000,in_port=LOCAL,eth_type=0x86dd,nw_proto=58,icmpv6_type=136,eth_dst=ee:ee:ee:e2:01:01,ipv6_dst=fd00:fec2::1f:e2,nd_target=fe80::ba2a:72ff:fee0:ff26,load:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],load:0xb82a72e0ff26->NXM_OF_ETH_SRC[],load:NXM_NX_IPV6_SRC[]->NXM_NX_IPV6_DST[],load:NXM_NX_ND_TARGET[]->NXM_NX_IPV6_SRC[],load:NXM_NX_ND_TARGET[]->NXM_NX_ND_TARGET[],load:0xb82a72e0ff26->NXM_NX_ND_TLL[],output:NXM_OF_IN_PORT[]),mod_dl_src:ee:ee:ee:e2:01:01,mod_dl_dst:33:33:ff:e0:ff:26,load:0x1f00e2->NXM_NX_IPV6_SRC[0..63],load:0xfd00fec200000000->NXM_NX_IPV6_SRC[64..127],load:0x1ffe0ff26->NXM_NX_IPV6_DST[0..63],load:0xff02000000000000->NXM_NX_IPV6_DST[64..127],load:0xba2a72fffee0ff26->NXM_NX_ND_TARGET[0..63],load:0xfe80000000000000->NXM_NX_ND_TARGET[64..127],load:0xeeeeeee20101->NXM_NX_ND_SLL[],LOCAL`,
		`cookie=0x0, duration=56818.582s, table=0, n_packets=27748, n_bytes=8362402, idle_age=40, priority=29300,tcp,in_port=31,nw_dst=169.254.169.254,tp_dst=80 actions=learn(table=12,idle_timeout=30,priority=10000,in_port=LOCAL,eth_type=0x800,nw_proto=6,ip_dst=100.95.86.79,tcp_src=9949,load:NXM_OF_ETH_DST[]->NXM_OF_ETH_SRC[],load:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],load:NXM_OF_IP_DST[]->NXM_OF_IP_SRC[],load:NXM_OF_IP_SRC[]->NXM_OF_IP_DST[],load:0x50->NXM_OF_TCP_SRC[],output:NXM_OF_IN_PORT[]),mod_dl_src:ee:ee:ee:5f:50:00,mod_dl_dst:b8:2a:72:e0:ff:26,mod_nw_src:100.95.86.79,mod_nw_dst:10.127.222.112,mod_tp_dst:9949,LOCAL`,
		`cookie=0x0, duration=6057.979s, table=0, n_packets=0, n_bytes=0, priority=29301,tcp6,in_port=31,ipv6_dst=fd00:ec2::254,tp_dst=80 actions=learn(table=12,idle_timeout=30,priority=10000,in_port=LOCAL,eth_type=0x86dd,nw_proto=6,ipv6_dst=fd00:fec2::1f:fa,tcp_src=9949,load:NXM_OF_ETH_DST[]->NXM_OF_ETH_SRC[],load:NXM_OF_ETH_SRC[]->NXM_OF_ETH_DST[],load:NXM_NX_IPV6_DST[]->NXM_NX_IPV6_SRC[],load:NXM_NX_IPV6_SRC[]->NXM_NX_IPV6_DST[],load:0x50->NXM_OF_TCP_SRC[],output:NXM_OF_IN_PORT[]),mod_dl_src:ee:ee:ee:e2:01:01,mod_dl_dst:b8:2a:72:e0:ff:26,load:0x1f00fa->NXM_NX_IPV6_SRC[0..63],load:0xfd00fec200000000->NXM_NX_IPV6_SRC[64..127],load:0xba2a72fffee0ff26->NXM_NX_IPV6_DST[0..63],load:0xfe80000000000000->NXM_NX_IPV6_DST[64..127],mod_tp_dst:9949,LOCAL`,
	}
	for _, c := range cases {
		f := &ovs.Flow{}
		err := f.UnmarshalText([]byte(c))
		if err != nil {
			t.Errorf("unmarshal %s fail %s", c, err)
		} else {
			flowStr, err := f.MarshalText()
			if err != nil {
				t.Errorf("marshal %s fail %s", c, err)
			} else {
				nf := &ovs.Flow{}
				err := nf.UnmarshalText(flowStr)
				if err != nil {
					t.Errorf("unmarshal %s fail %s", string(flowStr), err)
				} else {
					flowStr2, err := nf.MarshalText()
					if err != nil {
						t.Errorf("marshal %s fail %s", c, err)
					} else if string(flowStr2) != string(flowStr) {
						t.Errorf("flow1 != flow2: %s != %s", string(flowStr), string(flowStr2))
					} else {
						t.Logf("flow %s", string(flowStr2))
					}
				}
			}
		}
	}
}
