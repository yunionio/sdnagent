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
	"io/ioutil"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/digitalocean/go-openvswitch/ovs"
	"github.com/vishvananda/netlink"

	"yunion.io/x/log"

	"yunion.io/x/sdnagent/pkg/agent/utils"
)

type brIfaceMap map[string]map[string]struct{}

func (m brIfaceMap) has(br, iface string) bool {
	ifaces, ok := m[br]
	if !ok {
		return false
	}
	_, ok = ifaces[iface]
	return ok
}

func (m brIfaceMap) add(br, iface string) {
	ifaces, ok := m[br]
	if !ok {
		m[br] = map[string]struct{}{
			iface: struct{}{},
		}
		return
	}
	ifaces[iface] = struct{}{}
	return
}

func (m brIfaceMap) Copy() brIfaceMap {
	cp := brIfaceMap{}
	for br, ifaces := range m {
		ifaces_ := map[string]struct{}{}
		for iface, v := range ifaces {
			ifaces_[iface] = v
		}
		cp[br] = ifaces_
	}
	return cp
}

func (m brIfaceMap) mergeWith(m1 brIfaceMap) {
	for br, ifaces := range m1 {
		for iface := range ifaces {
			m.add(br, iface)
		}
	}
}

type ifaceJanitor struct{}

func newIfaceJanitor() *ifaceJanitor {
	return &ifaceJanitor{}
}

func (ij *ifaceJanitor) Start(ctx context.Context) {
	wg := ctx.Value("wg").(*sync.WaitGroup)
	defer wg.Done()

	scanTicker := time.NewTicker(IfaceJanitorInterval)
	defer scanTicker.Stop()
	for {
		select {
		case <-scanTicker.C:
			err := ij.scan(ctx)
			if err != nil {
				log.Errorf("scan: %s", err)
			}
		case <-ctx.Done():
			log.Infof("iface janitor bye")
			return
		}
	}
}

func (ij *ifaceJanitor) scan(ctx context.Context) error {
	hc, err := utils.NewHostConfig(DefaultHostConfigPath)
	if err != nil {
		return err
	}
	infraMap := brIfaceMap{}
	{
		for _, n := range hc.Networks {
			infraMap.add(n.Bridge, n.Ifname)
		}
	}
	gotMap := brIfaceMap{}
	{
		cli := ovs.New().VSwitch
		for br := range infraMap {
			ports, err := cli.ListPorts(br)
			if err != nil {
				return fmt.Errorf("ovs-vsctl list-ports %s: %s", br, err)
			}
			for _, port := range ports {
				gotMap.add(br, port)
			}
		}
	}
	wantMap := infraMap.Copy()
	{
		serversMap, err := ij.scanDescs(hc)
		if err != nil {
			return err
		}
		wantMap.mergeWith(serversMap)
	}
	for br, ifaces := range gotMap {
		for iface, _ := range ifaces {
			if !wantMap.has(br, iface) {
				ij.tryDestroy(br, iface)
			}
		}
	}
	return nil
}

func (ij *ifaceJanitor) scanDescs(hc *utils.HostConfig) (brIfaceMap, error) {
	serversPath := hc.ServersPath
	fis, err := ioutil.ReadDir(serversPath)
	if err != nil {
		return nil, fmt.Errorf("scan servers path %s: %s", serversPath, err)
	}
	r := brIfaceMap{}
	for _, fi := range fis {
		if !fi.IsDir() {
			continue
		}
		id := fi.Name()
		if REGEX_UUID.MatchString(id) {
			path := path.Join(serversPath, id)
			ug := utils.Guest{
				Id:         id,
				Path:       path,
				HostConfig: hc,
			}
			err := ug.LoadDesc()
			if err != nil {
				return nil, fmt.Errorf("load desc %s: %s", path, err)
			}
			for _, nic := range ug.NICs {
				r.add(nic.Bridge, nic.IfnameHost)
			}
		}
	}
	return r, nil
}

func (ij *ifaceJanitor) tryDestroy(br, iface string) {
	msgs := []string{}
	defer func() {
		// it's error, no matter what
		msg := strings.Join(msgs, "")
		log.Errorf("destroy %q %q: %s", br, iface, msg)
	}()
	var lk netlink.Link
	{
		var err error
		lk, err = netlink.LinkByName(iface)
		if err != nil {
			msgs = append(msgs, fmt.Sprintf("[LinkByName %s]", err))
		}
	}
	if lk != nil {
		// iface driver must be either tap or veth
		lkType := lk.Type()
		msgs = append(msgs, fmt.Sprintf("[linkType=%q]", lkType))
		switch lkType {
		case "veth", "tun":
		default:
			return
		}
	}
	cli := ovs.New().VSwitch
	err := cli.DeletePort(br, iface)
	if err == nil {
		err = fmt.Errorf("deleted")
	}
	msgs = append(msgs, fmt.Sprintf("[del-port %q %q: %s]", br, iface, err))

	if lk != nil {
		err := netlink.LinkDel(lk)
		if err == nil {
			err = fmt.Errorf("deleted")
		}
		msgs = append(msgs, fmt.Sprintf("[LinkDel %q: %s]", iface, err))
	}
	return
}
