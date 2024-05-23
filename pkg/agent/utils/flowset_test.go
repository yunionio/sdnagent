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

func flowStr(f *ovs.Flow) string {
	fstr, _ := f.MarshalText()
	return string(fstr)
}

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
