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
)

var (
	errNotRunning   = fmt.Errorf("not running")
	errPortNotReady = fmt.Errorf("port not ready") // no port is ready
	errSlaveMachine = fmt.Errorf("slave machine")
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

// refreshPortNo updates openflow port number for each guest's nic.  Returns
// true if all nics' port numbers are correctly updated, false otherwise, which
// usually caused by nic port is not yet in the bridge
func (g *Guest) refreshPortNo(ctx context.Context) bool {
	someOk := false
	for _, nic := range g.NICs {
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
	if g.IsSlave() {
		err = errSlaveMachine
		setPending = false
		return
	}
	someOk := g.refreshPortNo(ctx)
	if !someOk {
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

// TODO log
func (g *Guest) updateFlows(ctx context.Context) (err error) {
	bfs, err := g.FlowsMap()
	for bridge, flows := range bfs {
		flowman := g.watcher.agent.GetFlowMan(bridge)
		flowman.updateFlows(ctx, g.Who(), flows)
	}
	return
}

func (g *Guest) deleteFlows(ctx context.Context) {
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
	data := []*utils.TcData{}
	for _, nic := range g.NICs {
		d := nic.TcData()
		data = append(data, d)
	}
	g.watcher.tcMan.AddIfaces(ctx, g.Who(), data)
}

func (g *Guest) clearTc(ctx context.Context) {
	g.watcher.tcMan.ClearIfaces(ctx, g.Who())
}

func (g *Guest) updateOvn(ctx context.Context) {
	if len(g.VpcNICs) > 0 {
		ovnMan := g.watcher.ovnMan
		ovnMan.SetHostId(ctx, g.HostId)
		ovnMan.SetGuestNICs(ctx, g.Id, g.VpcNICs)
	}
}

func (g *Guest) clearOvn(ctx context.Context) {
	ovnMan := g.watcher.ovnMan
	ovnMan.SetGuestNICs(ctx, g.Id, nil)
}

func (g *Guest) UpdateSettings(ctx context.Context) {
	{
		err := g.refresh(ctx)
		switch err {
		case nil:
			g.updateFlows(ctx)
			g.updateTc(ctx)
		case errNotRunning, errPortNotReady, errSlaveMachine:
			g.clearSettings(ctx)
		}
	}
	g.updateOvn(ctx)
}

func (g *Guest) clearSettings(ctx context.Context) {
	g.deleteFlows(ctx)
	g.clearTc(ctx)
}

func (g *Guest) ClearSettings(ctx context.Context) {
	g.clearSettings(ctx)
	g.clearOvn(ctx)
	return
}
