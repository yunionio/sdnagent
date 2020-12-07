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
	"net"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/digitalocean/go-openvswitch/ovs"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"

	"yunion.io/x/onecloud/pkg/appsrv"
	"yunion.io/x/onecloud/pkg/cloudcommon/app"
	common_options "yunion.io/x/onecloud/pkg/cloudcommon/options"
	"yunion.io/x/onecloud/pkg/hostman/metadata"
	"yunion.io/x/onecloud/pkg/util/iproute2"
	"yunion.io/x/onecloud/pkg/vpcagent/ovn/mac"

	"yunion.io/x/sdnagent/pkg/agent/utils"
)

type mdDescGetter struct {
	netId   string
	watcher *serversWatcher
}

func (g *mdDescGetter) Get(ip string) jsonutils.JSONObject {
	guestDesc := g.watcher.FindGuestDescByNetIdIP(g.netId, ip)
	return guestDesc
}

const (
	metadataServerIP     = "169.254.169.254"
	metadataServerIPMask = "169.254.169.254/32"
)

func ovnMdVethPair(netId string) (local, peer string) {
	local = "md-" + netId[:(15-3-2)]
	peer = local + "-p"
	return local, peer
}

func ovnMdIsVethPeer(name string) bool {
	return strings.HasPrefix(name, "md-") && strings.HasSuffix(name, "-p")
}

type ovnMdServer struct {
	netId   string
	netCidr string
	watcher *serversWatcher

	ipNii map[string]netIdIP

	ns     netns.NsHandle
	origNs netns.NsHandle
	app    *appsrv.Application
}

func newOvnMdServer(netId, netCidr string, watcher *serversWatcher) *ovnMdServer {
	s := &ovnMdServer{
		netId:   netId,
		netCidr: netCidr,
		watcher: watcher,

		ns:     netns.None(),
		origNs: netns.None(),
		ipNii:  map[string]netIdIP{},
	}
	return s
}

func (s *ovnMdServer) Start(ctx context.Context) {
	if err := s.ensureMdInfra(ctx); err != nil {
		log.Errorf("ensureMdInfra: %v", err)
	}
	if err := s.nsRun(ctx, func(ctx context.Context) error {
		svc := &metadata.Service{
			Address: metadataServerIP,
			Port:    80,

			DescGetter: &mdDescGetter{
				netId:   s.netId,
				watcher: s.watcher,
			},
		}
		dbAccess := false
		s.app = app.InitApp(&common_options.BaseOptions{
			ApplicationID:      fmt.Sprintf("metadata-server-4-subnet-%s", s.netId),
			RequestWorkerCount: 1,
		}, dbAccess)
		metadata.Start(s.app, svc)

		return nil
	}); err != nil {
		log.Errorf("ovnMd: serve %s: %v", s.netId, err)
	}
}

func (s *ovnMdServer) Stop(ctx context.Context) {
	log.Warningf("ovnMd off %s", s.netId)
	if s.app != nil {
		ctx, _ = context.WithTimeout(ctx, 3*time.Second)
		if err := s.app.Stop(ctx); err != nil {
			log.Errorf("ovnMd: stop appsrv %s", s.netId)
		}
	}
	if err := s.ensureMdInfraOff(ctx); err != nil {
		log.Errorf("ensureMdInfra: %v", err)
	}
}

func (s *ovnMdServer) ensureMdInfra(ctx context.Context) error {
	var (
		ns          = s.nsName()
		lsp         = s.ovnLspName()
		local, peer = s.vethPair()
		localMacStr = mac.HashSubnetMetadataMac(s.netId)
		bridge      = s.integrationBridge()
	)
	{ // create netns
		var (
			err error
			wg  = &sync.WaitGroup{}
		)
		wg.Add(1)
		go func() {
			defer wg.Done()

			runtime.LockOSThread()
			defer runtime.UnlockOSThread()

			s.origNs, err = netns.Get()
			if err != nil {
				err = errors.Wrap(err, "get current ns")
			}
			s.ns, err = netns.New()
			//s.ns, err = netns.NewNamed(ns)
			if err != nil {
				err = errors.Wrapf(err, "new netns %q", ns)
			}

			if err = netns.Set(s.origNs); err != nil {
				err = errors.Wrap(err, "set to orig ns")
			}
		}()
		wg.Wait()
		if err != nil {
			// we have reason to panic
			return err
		}
	}
	{ // add veth pair
		veth := &netlink.Veth{}
		veth.Name = local
		veth.PeerName = peer
		if err := netlink.LinkAdd(veth); err != nil {
			return errors.Wrapf(err, "add veth pair %q, %q", local, peer)
		}
	}
	{ // setup local
		if l, err := netlink.LinkByName(local); err != nil {
			return errors.Wrapf(err, "setup local: get link %q", local)
		} else if err = netlink.LinkSetNsFd(l, int(s.ns)); err != nil {
			return errors.Wrapf(err, "setup local: set link ns %q", local)
		}
		if err := s.nsRun(ctx, func(ctx context.Context) error {
			if err := iproute2.NewLink(local).Address(localMacStr).Up().Err(); err != nil {
				return errors.Wrapf(err, "set veth local link %q", local)
			}
			if err := iproute2.NewAddress(local, metadataServerIPMask).Exact().Err(); err != nil {
				return errors.Wrapf(err, "set veth local address %q", local)
			}
			if err := iproute2.NewRoute(local).AddByCidr(s.netCidr, "").Err(); err != nil {
				return errors.Wrapf(err, "add net cidr %q", s.netCidr)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	{ // setup brvpc
		args := []string{
			"ovs-vsctl",
			"--", "--may-exist", "add-port", bridge, peer,
			"--", "set", "Interface", peer, "external_ids:iface-id=" + lsp,
		}
		if err := utils.RunOvsctl(ctx, args); err != nil {
			return err
		}
		if err := iproute2.NewLink(peer).Up().Err(); err != nil {
			return errors.Wrapf(err, "set link up")
		}
	}
	return nil
}

func (s *ovnMdServer) ensureMdInfraOff(ctx context.Context) error {
	var (
		_, peer = s.vethPair()
		bridge  = s.integrationBridge()
	)

	// release network namespace
	for _, nsh := range []netns.NsHandle{
		s.ns, s.origNs,
	} {
		if int(nsh) >= 0 {
			nsh.Close()
		}
	}
	{ // cleanup bridge
		args := []string{
			"ovs-vsctl",
			"--", "--if-exists", "del-port", bridge, peer,
		}
		if err := utils.RunOvsctl(ctx, args); err != nil {
			return err
		}
	}
	{ // cleanup veth
		link, err := netlink.LinkByName(peer)
		if err == nil {
			err = netlink.LinkDel(link)
		}
		if err != nil {
			if _, ok := err.(netlink.LinkNotFoundError); !ok {
				return err
			}
		}
	}
	return nil
}

func (s *ovnMdServer) nsRun(ctx context.Context, f func(ctx context.Context) error) error {
	var (
		wg  = &sync.WaitGroup{}
		err error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		netns.Set(s.ns)
		defer netns.Set(s.origNs)

		err = f(ctx)

	}()
	wg.Wait()
	return err
}

func (s *ovnMdServer) integrationBridge() string {
	return s.watcher.hostConfig.OvnIntegrationBridge
}

func (s *ovnMdServer) nsName() string {
	return "subnet-md-" + s.netId
}

func (s *ovnMdServer) ovnLspName() string {
	return "subnet-md/" + s.netId
}

func (s *ovnMdServer) vethPair() (string, string) {
	return ovnMdVethPair(s.netId)
}

func (s *ovnMdServer) NoteOff(ctx context.Context, nii netIdIP) bool {
	delete(s.ipNii, nii.ip)
	return len(s.ipNii) == 0
}

func (s *ovnMdServer) NoteOn(ctx context.Context, nii netIdIP) {
	s.ipNii[nii.ip] = nii
}

type netIdIP struct {
	netId   string
	ip      string
	masklen int

	guestId string
}

func (nii netIdIP) key() string {
	return nii.netId + "/" + nii.ip
}

type netIdIPm map[string]netIdIP

func newNetIdIPm(nics []*utils.GuestNIC) netIdIPm {
	niim := netIdIPm{}
	for _, nic := range nics {
		nii := netIdIP{
			netId:   nic.NetId,
			ip:      nic.IP,
			masklen: nic.Masklen,
		}
		niim[nii.key()] = nii
	}
	return niim
}

func (niim netIdIPm) List() (r []netIdIP) {
	for _, nii := range niim {
		r = append(r, nii)
	}
	return
}

func (niim netIdIPm) Diff(niim1 netIdIPm) (dels, adds []netIdIP) {
	for _, nii := range niim {
		if _, ok := niim1[nii.key()]; !ok {
			dels = append(dels, nii)
		}
	}
	for _, nii1 := range niim1 {
		if _, ok := niim[nii1.key()]; !ok {
			adds = append(adds, nii1)
		}
	}
	return
}

type ovnMdReq struct {
	guestId string
	nics    []*utils.GuestNIC
	r       chan utils.Empty
}

type ovnMdMan struct {
	watcher *serversWatcher
	c       chan *ovnMdReq

	guestNics map[string]netIdIPm     // key: guestId
	mdServers map[string]*ovnMdServer // key: netId
}

func newOvnMdMan(watcher *serversWatcher) *ovnMdMan {
	man := &ovnMdMan{
		watcher: watcher,
		c:       make(chan *ovnMdReq),

		mdServers: map[string]*ovnMdServer{},
		guestNics: map[string]netIdIPm{},
	}
	return man
}

func (man *ovnMdMan) Start(ctx context.Context) {
	wg := ctx.Value("wg").(*sync.WaitGroup)
	defer wg.Done()

	refreshTicker := time.NewTicker(OvnMdManRefreshRate)
	defer refreshTicker.Stop()
	for {
		select {
		case req := <-man.c:
			var (
				dels, adds []netIdIP
			)
			niim1 := newNetIdIPm(req.nics)
			if niim, ok := man.guestNics[req.guestId]; ok {
				dels, adds = niim.Diff(niim1)
			} else {
				adds = niim1.List()
			}
			man.guestNics[req.guestId] = niim1

			for _, del := range dels {
				del.guestId = req.guestId
				man.noteOff(ctx, del)
			}
			for _, add := range adds {
				add.guestId = req.guestId
				man.noteOn(ctx, add)
			}
			req.r <- utils.Empty{}
		case <-refreshTicker.C:
			man.cleanup(ctx)
		case <-ctx.Done():
			log.Infof("ovnMd man bye")
			return
		}
	}
}

func (man *ovnMdMan) noteOff(ctx context.Context, nii netIdIP) {
	netId := nii.netId
	mdServer, ok := man.mdServers[netId]
	if ok {
		shouldStop := mdServer.NoteOff(ctx, nii)
		if shouldStop {
			mdServer.Stop(ctx)
			delete(man.mdServers, netId)
		}
	} else {
		log.Errorf("dec count error: %#v", nii)
	}
}

func (man *ovnMdMan) noteOn(ctx context.Context, nii netIdIP) {
	netId := nii.netId
	mdServer, ok := man.mdServers[netId]
	if !ok {
		ipmask := fmt.Sprintf("%s/%d", nii.ip, nii.masklen)
		_, ipnet, err := net.ParseCIDR(ipmask)
		if err != nil {
			log.Errorf("guestId %s, ip %s, parse cidr: %s", nii.guestId, nii.ip, ipmask)
		}
		netCidr := ipnet.String()
		mdServer = newOvnMdServer(netId, netCidr, man.watcher)
		go mdServer.Start(ctx)
		man.mdServers[netId] = mdServer
	}
	mdServer.NoteOn(ctx, nii)
}

func (man *ovnMdMan) SetGuestNICs(ctx context.Context, guestId string, nics []*utils.GuestNIC) {
	req := &ovnMdReq{
		guestId: guestId,
		nics:    nics,
		r:       make(chan utils.Empty),
	}
	select {
	case man.c <- req:
		select {
		case <-req.r:
		case <-ctx.Done():
		}
	case <-ctx.Done():
	}
}

func (man *ovnMdMan) cleanup(ctx context.Context) {
	defer log.Infoln("ovnMd: clean done")

	var (
		cli       = ovs.New().VSwitch
		br        = man.watcher.hostConfig.OvnIntegrationBridge
		peerWants []string
		peerGots  []string
	)

	// fill gots
	if ports, err := cli.ListPorts(br); err != nil {
		log.Errorf("ovnMd list bridges: %v", err)
		return
	} else {
		for _, port := range ports {
			if ovnMdIsVethPeer(port) {
				peerGots = append(peerGots, port)
			}
		}
	}
	// fill wants
	for netId := range man.mdServers {
		_, peer := ovnMdVethPair(netId)
		peerWants = append(peerWants, peer)
	}
	// cleanup
	for _, got := range peerGots {
		ok := false
		for _, want := range peerWants {
			if got == want {
				ok = true
				break
			}
		}
		if !ok {
			log.Warningf("clean port: %s", got)
			if err := cli.DeletePort(br, got); err != nil {
				log.Errorf("ovs delete port: %s %s: %v", br, got, err)
			}
			{
				link, err := netlink.LinkByName(got)
				if err == nil {
					err = netlink.LinkDel(link)
				}
				if err != nil {
					if _, ok := err.(netlink.LinkNotFoundError); !ok {
						log.Errorf("ovnMd: delete link %s: %v", got, err)
					}
				}
			}
		}
	}
}
