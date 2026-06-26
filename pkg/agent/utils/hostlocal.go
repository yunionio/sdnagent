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
	"net"
	"sync"

	"yunion.io/x/onecloud/pkg/appsrv"
)

var hostLocalMap *sync.Map

type HostLocal struct {
	HostConfig *HostConfig

	*HostConfigNetwork

	metadataPort int
	metadataApp  *appsrv.Application
}

func (h *HostLocal) String() string {
	prefix, mdIp, mdMac := h.fakeMdSrcIpMac(0)
	prefix6, mdIp6, mdMac6 := h.fakeMdSrcIp6Mac(0, "")
	return fmt.Sprintf("HostLocal: Bridge=%s, ifname=%s, IP=%s, IPLocal=%s, IP6=%s, IP6Local=%s, MAC=%s, fakeMdPrefix=%s fakeMdIp=%s fakeMdPrefix6=%s fakeMdSrcIpMac=%s fakeMdIp6=%s fakeMdSrcIp6Mac=%s", h.Bridge, h.Ifname, h.IP, h.IPLocal, h.IP6, h.IP6Local, h.mac, prefix, mdIp, mdMac, prefix6, mdIp6, mdMac6)
}

func (h *HostLocal) IP4() net.IP {
	if h.IP != nil {
		return h.IP
	}
	if h.IPLocal != nil {
		return h.IPLocal
	}
	return nil
}

func init() {
	hostLocalMap = &sync.Map{}
}

func FetchHostLocal(hl *HostLocal, watcher IServerWatcher) *HostLocal {
	if uhl, ok := hostLocalMap.Load(hl.Bridge); !ok {
		// not found, register
		go hl.StartMetadataServer(watcher)
		hostLocalMap.Store(hl.Bridge, hl)
		return hl
	} else {
		// find, to update fields
		nhl := uhl.(*HostLocal)
		nhl.HostConfig = hl.HostConfig
		nhl.HostLocalNets = hl.HostLocalNets
		return nhl
	}
}

func findHostLocalByBridge(bridge string) *HostLocal {
	if val, ok := hostLocalMap.Load(bridge); ok {
		return val.(*HostLocal)
	}
	return nil
}
