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
	masterIP, masterMAC, err := hl.watcher.hostConfig.MasterIPMAC()
	if err != nil {
		log.Errorf("get master ip/mac failed: %s", err)
		return
	}
	for _, hcn := range hl.watcher.hostConfig.Networks {
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
			MasterIP:   masterIP,
			MasterMAC:  masterMAC,
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
