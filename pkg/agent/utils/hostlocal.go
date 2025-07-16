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
	"net"
	"sync"

	"yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/appsrv"
)

var hostLocalMap *sync.Map

type HostLocal struct {
	HostConfig *HostConfig
	Bridge     string
	Ifname     string
	IP         net.IP
	IP6        net.IP
	IP6Local   net.IP
	MAC        net.HardwareAddr

	HostLocalNets []compute.NetworkDetails

	metadataPort int
	metadataApp  *appsrv.Application
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
