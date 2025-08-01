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

package server

import (
	"context"

	"yunion.io/x/log"

	"yunion.io/x/sdnagent/pkg/agent/utils"
)

type HostLocal struct {
	watcher *serversWatcher

	bridgeMap map[string]*utils.HostLocal
}

func NewHostLocal(watcher *serversWatcher) *HostLocal {
	return &HostLocal{
		watcher:   watcher,
		bridgeMap: make(map[string]*utils.HostLocal),
	}
}

func (hl *HostLocal) updateFlows(ctx context.Context) {
	for _, hcn := range hl.watcher.hostConfig.HostNetworkConfigs() {
		mac, err := hcn.MAC()
		if err != nil {
			log.Errorf("get ip/mac for %s failed: %s", hcn.Bridge, err)
			continue
		}
		hostLocal := &utils.HostLocal{
			HostConfig: hl.watcher.hostConfig,
			Bridge:     hcn.Bridge,
			Ifname:     hcn.Ifname,
			IP:         hcn.IP,
			IP6:        hcn.IP6,
			IP6Local:   hcn.IP6Local,
			MAC:        mac,

			HostLocalNets: hcn.HostLocalNets,
		}
		hostLocal = utils.FetchHostLocal(hostLocal, hl.watcher)

		flows, err := hostLocal.FlowsMap()
		if err != nil {
			log.Errorf("prepare %s hostlocal flows failed: %s", hcn.Bridge, err)
			continue
		}
		flowman := hl.watcher.agent.GetFlowMan(hcn.Bridge)
		flowman.updateFlows(ctx, hostLocal.Who(), flows[hcn.Bridge])

		if hostLocal.IP != nil {
			err = hostLocal.EnsureFakeLocalMetadataRoute()
			if err != nil {
				log.Errorf("EnsureFakeLocalMetadataRoute %s hostlocal flows failed: %s", hcn.Bridge, err)
				continue
			}
		}

		if hostLocal.IP6Local != nil {
			err = hostLocal.EnsureFakeLocalMetadataRoute6()
			if err != nil {
				log.Errorf("EnsureFakeLocalMetadataRoute6 %s hostlocal flows failed: %s", hcn.Bridge, err)
				continue
			}
		}
	}
}

func (hl *HostLocal) updateTc(ctx context.Context, sync bool) {
	if hl.watcher.tcMan == nil {
		return
	}
	data := []*utils.TcData{}
	for _, hcn := range hl.watcher.hostConfig.HostNetworkConfigs() {
		td := &utils.TcData{
			Type:   utils.TC_DATA_TYPE_HOSTLOCAL,
			Ifname: hcn.Ifname,
		}
		data = append(data, td)
	}
	hl.watcher.tcMan.AddIfaces(ctx, "hostlocal", data, sync)
}

func (hl *HostLocal) UpdateSettings(ctx context.Context, sync bool) {
	hl.updateFlows(ctx)
	hl.updateTc(ctx, sync)
}
