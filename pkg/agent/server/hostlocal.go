package server

import (
	"context"

	"github.com/yunionio/log"
	"yunion.io/yunion-sdnagent/pkg/agent/utils"
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
	for _, hcn := range hl.watcher.hostConfig.Networks {
		ip, err := hl.watcher.hostConfig.MasterIP()
		if err != nil {
			log.Errorf("get master ip failed; %s", err)
			continue
		}
		mac, err := hl.watcher.hostConfig.MasterMAC()
		if err != nil {
			log.Errorf("get master mac failed; %s", err)
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
	data := []*utils.TcData{}
	for _, hcn := range hl.watcher.hostConfig.Networks {
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
