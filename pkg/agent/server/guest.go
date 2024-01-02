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
	"fmt"
	"time"

	"github.com/digitalocean/go-openvswitch/ovs"

	"yunion.io/x/log"
	"yunion.io/x/sdnagent/pkg/agent/utils"

	"yunion.io/x/onecloud/pkg/mcclient/auth"
	mcclient_modules "yunion.io/x/onecloud/pkg/mcclient/modules/compute"
)

var (
	errNotRunning   = fmt.Errorf("not running")
	errPortNotReady = fmt.Errorf("port not ready") // no port is ready
	errVolatileHost = fmt.Errorf("volatile host")
)

type Guest struct {
	*utils.Guest
	watcher         *serversWatcher
	lastSeenPending *time.Time
}

func NewGuest(guest *utils.Guest, watcher *serversWatcher) *Guest {
	return &Guest{
		Guest:   guest,
		watcher: watcher,
	}
}

// refreshNicPortNo updates openflow port number for guest's each nic.  Returns
// true if all nics' port numbers are correctly updated, false otherwise, which
// usually caused by nic port is not yet in the bridge
func (g *Guest) refreshNicPortNo(ctx context.Context, nics []*utils.GuestNIC) bool {
	someOk := false
	for _, nic := range nics {
		bridge := nic.Bridge
		ifname := nic.IfnameHost
		portStats, err := utils.DumpPort(bridge, ifname)
		if err == nil {
			someOk = true
			nic.PortNo = portStats.PortID
		}
	}
	return someOk
}

func (g *Guest) reloadDesc(ctx context.Context) error {
	oldM := map[string]uint16{}
	oldNICs := g.NICs
	for _, nic := range oldNICs {
		if nic.CtZoneIdSet {
			oldM[nic.MAC] = nic.CtZoneId
		}
	}

	err := g.LoadDesc()
	if err != nil {
		return err
	}
	if g.NeedsSync() {
		go func() {
			// desc change will be picked up by watcher
			log.Infof("guest sync %s", g.Id)
			hc := g.watcher.hostConfig
			s := auth.GetAdminSession(ctx, hc.Region)
			_, err := mcclient_modules.Servers.PerformAction(s, g.Id, "sync", nil)
			if err != nil {
				log.Errorf("guest sync %s: %v", g.Id, err)
			}
		}()
	}

	for _, nic := range g.NICs {
		if i, ok := oldM[nic.MAC]; ok {
			delete(oldM, nic.MAC)
			nic.CtZoneId = i
			continue
		}
		zoneId, err := g.watcher.zoneMan.AllocateZoneId(nic.MAC)
		if err != nil {
			return fmt.Errorf("ct zone id allocation failed: %s", err)
		}
		nic.CtZoneId = zoneId
		nic.CtZoneIdSet = true
	}
	for mac, _ := range oldM {
		g.watcher.zoneMan.FreeZoneId(mac)
	}
	return nil
}

func (g *Guest) setPending() {
	if g.lastSeenPending == nil {
		now := time.Now()
		g.lastSeenPending = &now
	}
}

func (g *Guest) clearPending() {
	g.lastSeenPending = nil
}

func (g *Guest) IsPending() bool {
	if g.lastSeenPending == nil {
		return false
	}
	if time.Since(*g.lastSeenPending) < WatcherRecentPendingTime {
		return true
	}
	return false
}

func (g *Guest) refresh(ctx context.Context) (err error) {
	setPending := true
	defer func() {
		if err != nil {
			log.Warningf("update guest flows %s: %s", g.Id, err)
			if setPending {
				g.setPending()
			}
		} else {
			g.clearPending()
		}
	}()

	err = g.reloadDesc(ctx)
	if err != nil {
		return
	}
	if g.IsVolatileHost() {
		err = errVolatileHost
		setPending = false
		return
	}
	// serve if any nics are ready
	someOk0 := g.refreshNicPortNo(ctx, g.NICs)
	someOk1 := g.refreshNicPortNo(ctx, g.VpcNICs)
	if !someOk0 && !someOk1 {
		if g.IsVM() && !g.Running() {
			// we will be notified when its pid is to be updated
			// so there is no need to set pending for it now
			err = errNotRunning
			setPending = false
		} else {
			// NOTE crashed container can make pending watcher busy
			err = errPortNotReady
		}
		// next we will clean flow rules for them
	}
	return
}

func (g *Guest) updateClassicFlows(ctx context.Context) (err error) {
	bfs, err := g.FlowsMap()
	for bridge, flows := range bfs {
		flowman := g.watcher.agent.GetFlowMan(bridge)
		flowman.updateFlows(ctx, g.Who(), flows)
	}
	return
}

func (g *Guest) clearClassicFlows(ctx context.Context) {
	bridges := map[string]bool{}
	for _, nic := range g.NICs {
		bridges[nic.Bridge] = true
		g.watcher.zoneMan.FreeZoneId(nic.MAC)
	}
	for bridge, _ := range bridges {
		flowman := g.watcher.agent.GetFlowMan(bridge)
		flowman.updateFlows(ctx, g.Who(), []*ovs.Flow{})
	}
	g.clearPending()
}

func (g *Guest) updateTc(ctx context.Context) {
	if g.watcher.tcMan == nil {
		return
	}
	data := []*utils.TcData{}
	for _, nic := range g.NICs {
		d := nic.TcData()
		data = append(data, d)
	}
	g.watcher.tcMan.AddIfaces(ctx, g.Who(), data)
}

func (g *Guest) clearTc(ctx context.Context) {
	if g.watcher.tcMan == nil {
		return
	}
	g.watcher.tcMan.ClearIfaces(ctx, g.Who())
}

func (g *Guest) updateOvn(ctx context.Context) {
	if g.HostConfig.DisableLocalVpc {
		return
	}

	if len(g.VpcNICs) > 0 && g.HostId != "" {
		ovnMan := g.watcher.ovnMan
		ovnMan.SetHostId(ctx, g.HostId)
		ovnMan.SetGuestNICs(ctx, g.Id, g.VpcNICs)

		ovnMdMan := g.watcher.ovnMdMan
		ovnMdMan.SetGuestNICs(ctx, g.Id, g.VpcNICs)
	}
}

func (g *Guest) clearOvn(ctx context.Context) {
	if g.HostConfig.DisableLocalVpc {
		return
	}

	ovnMan := g.watcher.ovnMan
	ovnMan.SetGuestNICs(ctx, g.Id, nil)

	ovnMdMan := g.watcher.ovnMdMan
	ovnMdMan.SetGuestNICs(ctx, g.Id, nil)
}

func (g *Guest) UpdateSettings(ctx context.Context) {
	err := g.refresh(ctx)
	switch err {
	case nil:
		g.updateClassicFlows(ctx)
		g.updateTc(ctx)
		g.updateOvn(ctx)
		if g.HostId != "" {
			g.watcher.agent.HostId(g.HostId)
		}
	case errNotRunning, errPortNotReady, errVolatileHost:
		log.Debugf("guest %s(%s) ClearSettings due to g.refresh %s", g.Name, g.Id, err)
		g.ClearSettings(ctx)
	default:
		log.Errorf("guest %s(%s) g.refresh error %s", g.Name, g.Id, err)
	}
}

func (g *Guest) ClearSettings(ctx context.Context) {
	g.clearClassicFlows(ctx)
	g.clearTc(ctx)
	g.clearOvn(ctx)
}
