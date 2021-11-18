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
}

func NewHostLocal(watcher *serversWatcher) *HostLocal {
	return &HostLocal{
		watcher: watcher,
	}
}

func (hl *HostLocal) updateFlows(ctx context.Context) {
	for _, hcn := range hl.watcher.hostConfig.HostNetworkConfigs() {
		ip, mac, err := hcn.IPMAC()
		if err != nil {
			log.Errorf("get ip/mac for %s failed: %s", hcn.Bridge, err)
			continue
		}
		hostLocal := &utils.HostLocal{
			HostConfig: hl.watcher.hostConfig,
			Bridge:     hcn.Bridge,
			Ifname:     hcn.Ifname,
			IP:         ip,
			MAC:        mac,
		}
		flows, err := hostLocal.FlowsMap()
		if err != nil {
			log.Errorf("prepare %s hostlocal flows failed: %s", hcn.Bridge, err)
			continue
		}
		flowman := hl.watcher.agent.GetFlowMan(hcn.Bridge)
		flowman.updateFlows(ctx, hostLocal.Who(), flows[hcn.Bridge])
	}
}

func (hl *HostLocal) updateTc(ctx context.Context) {
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
	hl.watcher.tcMan.AddIfaces(ctx, "hostlocal", data)
}

func (hl *HostLocal) UpdateSettings(ctx context.Context) {
	hl.updateFlows(ctx)
	hl.updateTc(ctx)
}
