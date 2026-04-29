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
	"fmt"

	"yunion.io/x/sdnagent/pkg/tc"
)

const (
	OvsLocalPortNo = 65534 // reference: https://github.com/openvswitch/ovs/blob/master/lib/netdev-offload.c#L1214

	DefaultHostHtbRate = 1000 * 1000 * 1000 * 1000 // 1000Gbps

	ingressAmplifier = 1.0
	egressAmplifier  = 1.1
)

type TcData struct {
	Ifname      string `json:"ifname"`
	IngressMbps uint64 `json:"ingress_mbps"`
	EgressMbps  uint64 `json:"egress_mbps"`

	Bridge string `json:"bridge"`
	PortNo int    `json:"port_no"`
}

func (td TcData) IfbIfname() string {
	return "r" + td.Ifname
}

func (td *TcData) guestNicRootQdisc() []tc.IQdisc {
	return []tc.IQdisc{
		td.tbfQdisc(uint64(float64(td.IngressMbps) * ingressAmplifier)),
		td.ingressQdisc(),
	}
}

func (td *TcData) guestIfbNicRootQdisc() []tc.IQdisc {
	return []tc.IQdisc{
		td.tbfQdisc(uint64(float64(td.EgressMbps) * egressAmplifier)),
	}
}

func (td *TcData) tbfQdisc(rateMbps uint64) *tc.QdiscTbf {
	rates := rateMbps * 1000 * 1000
	bytesPerSec := rates / 8
	burst := bytesPerSec / 1000
	if burst < 3400 {
		burst = 3400
	}
	latency := 100000000 // 100ms
	return &tc.QdiscTbf{
		SBaseTcQdisc: &tc.SBaseTcQdisc{
			Kind:   "tbf",
			Handle: "1:",
			Parent: "",
			Root:   true,
		},
		Rate:    rates,
		Burst:   burst,
		Latency: uint64(latency),
	}
}

func (td *TcData) ingressQdisc() *tc.QdiscIngress {
	return &tc.QdiscIngress{
		SBaseTcQdisc: &tc.SBaseTcQdisc{
			Kind:   "ingress",
			Handle: "ffff:",
			Parent: "",
			Root:   false,
		},
	}
}

func (td *TcData) hostRootQdisc() []tc.IQdisc {
	return []tc.IQdisc{
		&tc.QdiscHtb{
			SBaseTcQdisc: &tc.SBaseTcQdisc{
				Kind:   "htb",
				Handle: "1:",
				Parent: "",
				Root:   true,
			},
			DefaultClass: OvsLocalPortNo,
		},
	}
}

func (td *TcData) hostRootClass() []tc.IClass {
	rootClass := &tc.SHtbClass{
		SBaseTcClass: &tc.SBaseTcClass{
			Kind:    "htb",
			ClassId: "1:1",
			Parent:  nil,
		},
		Rate: DefaultHostHtbRate,
		Ceil: DefaultHostHtbRate,
	}
	defaultClass := &tc.SHtbClass{
		SBaseTcClass: &tc.SBaseTcClass{
			Kind:    "htb",
			ClassId: fmt.Sprintf("1:%x", OvsLocalPortNo),
			Parent:  rootClass,
		},
		Rate: DefaultHostHtbRate,
		Ceil: DefaultHostHtbRate,
	}
	return []tc.IClass{
		rootClass,
		defaultClass,
	}
}

func (td *TcData) guestNicClass() []tc.IClass {
	return []tc.IClass{
		/* &tc.SHtbClass{
			SBaseTcClass: &tc.SBaseTcClass{
				Kind:    "htb",
				ClassId: fmt.Sprintf("1:%x", td.PortNo),
				Parent:  hostRootClass,
			},
			Rate: td.EgressMbps * 1000 * 1000,
			Ceil: td.EgressMbps * 1000 * 1000,
		}, */
	}
}

func (td *TcData) guestNicFilter() []tc.IFilter {
	return []tc.IFilter{
		/* &tc.SFwFilter{
			SBaseTcFilter: &tc.SBaseTcFilter{
				Kind:     "fw",
				Parent:   hostRootQdisc,
				Prio:     1,
				Protocol: "ip",
			},
			ClassId: fmt.Sprintf("1:%x", td.PortNo),
			Handle:  uint32(td.PortNo),
		}, */
		&tc.SU32Filter{
			SBaseTcFilter: &tc.SBaseTcFilter{
				Kind:     "u32",
				Parent:   td.ingressQdisc(),
				Prio:     49152,
				Protocol: "ip",
			},
			RedirectDev: td.IfbIfname(),
		},
	}
}

func (td *TcData) GuestQdiscTree() *tc.QdiscTree {
	return tc.NewQdiscTree(td.guestNicRootQdisc(), nil, td.guestNicFilter())
}

func (td *TcData) GuestIfbQdiscTree() *tc.QdiscTree {
	return tc.NewQdiscTree(td.guestIfbNicRootQdisc(), nil, nil)
}

func (td *TcData) HostRootQdiscTree() *tc.QdiscTree {
	return tc.NewQdiscTree(td.hostRootQdisc(), td.hostRootClass(), nil)
}

func (td *TcData) HostGuestQdiscTree(rootCls tc.IClass, rootQdisc tc.IQdisc) *tc.QdiscTree {
	return tc.NewQdiscTree(nil, nil, nil)
}
