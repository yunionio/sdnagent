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

	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/digitalocean/go-openvswitch/ovs"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"

	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"

	"yunion.io/x/onecloud/pkg/appsrv"
	"yunion.io/x/onecloud/pkg/cloudcommon/app"
	common_options "yunion.io/x/onecloud/pkg/cloudcommon/options"
	"yunion.io/x/onecloud/pkg/hostman/guestman/desc"
	fwdpb "yunion.io/x/onecloud/pkg/hostman/guestman/forwarder/api"
	"yunion.io/x/onecloud/pkg/hostman/metadata"
	"yunion.io/x/onecloud/pkg/util/iproute2"
	"yunion.io/x/onecloud/pkg/vpcagent/ovn/mac"

	"yunion.io/x/sdnagent/pkg/agent/utils"
)

type mdDescGetter struct {
	netId   string
	watcher *serversWatcher
}

func (g *mdDescGetter) Get(ip string) *desc.SGuestDesc {
	guestDesc := g.watcher.FindGuestDescByNetIdIP(g.netId, ip)
	return guestDesc
}

func ovnMdVethPair(netId string) (local, peer string) {
	local = "md-" + netId[:(15-3-2)]
	peer = local + "-p"
	return local, peer
}

func ovnMdIsVethPeer(name string) bool {
	return strings.HasPrefix(name, "md-") && strings.HasSuffix(name, "-p")
}

type ovnMdServerReqCmd int

const (
	ovnMdServerReqCmdOpenForward ovnMdServerReqCmd = iota
	ovnMdServerReqCmdListForward
	ovnMdServerReqCmdCloseForward
)

type ovnMdServerReq struct {
	Cmd   ovnMdServerReqCmd
	Data  interface{}
	RespC chan<- interface{}
}

type ovnMdServer struct {
	netId    string
	netCidr  string
	netCidr6 string
	watcher  *serversWatcher

	ipNii map[string]netIdIP

	ns  netns.NsHandle
	app *appsrv.Application

	requestC          chan *ovnMdServerReq
	forwardCtx        context.Context
	forwardCancelFunc context.CancelFunc
}

func newOvnMdServer(netId, netCidr, netCidr6 string, watcher *serversWatcher) *ovnMdServer {
	s := &ovnMdServer{
		netId:    netId,
		netCidr:  netCidr,
		netCidr6: netCidr6,
		watcher:  watcher,

		ns:    netns.None(),
		ipNii: map[string]netIdIP{},

		requestC: make(chan *ovnMdServerReq),
	}
	return s
}

func (s *ovnMdServer) Start(ctx context.Context) {
	if err := s.ensureMdInfra(ctx); err != nil {
		log.Errorf("ensureMdInfra: %v", err)
	}
	{
		s.forwardCtx, s.forwardCancelFunc = context.WithCancel(ctx)
		go s.serveRequest(ctx)
	}
	if err := s.nsRun(ctx, func(ctx context.Context) error {
		svc := &metadata.Service{
			Address: "", // listen to all addresses, both IPv4 and IPv6
			Port:    80,

			DescGetter: &mdDescGetter{
				netId:   s.netId,
				watcher: s.watcher,
			},
		}
		dbAccess := false
		s.app = app.InitApp(&common_options.BaseOptions{
			ApplicationID:          fmt.Sprintf("metadata-server-4-subnet-%s", s.netId),
			RequestWorkerCount:     4,
			RequestWorkerQueueSize: 128,
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
	s.forwardCancelFunc()
	if err := s.ensureMdInfraOff(ctx); err != nil {
		log.Errorf("ensureMdInfraOff: %v", err)
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

			err = s.nsRun_(ctx, func(ctx context.Context) error {
				s.ns, err = netns.New()
				if err != nil {
					return errors.Wrapf(err, "new netns %q", ns)
				}
				return nil
			})
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
			var metadataServerIPMasks []string
			for _, addr := range s.watcher.hostConfig.MetadataServerIp4s {
				metadataServerIPMasks = append(metadataServerIPMasks, fmt.Sprintf("%s/32", addr))
			}
			for _, addr := range s.watcher.hostConfig.MetadataServerIp6s {
				metadataServerIPMasks = append(metadataServerIPMasks, fmt.Sprintf("%s/128", addr))
			}
			if err := iproute2.NewAddress(local, metadataServerIPMasks...).Exact().Err(); err != nil {
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
	if int(s.ns) >= 0 {
		s.ns.Close()
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
	return s.nsRun_(ctx, func(ctx context.Context) error {
		if err := netns.Set(s.ns); err != nil {
			return errors.Wrapf(err, "nsRun: set netns %s", s.ns)
		}
		return f(ctx)
	})
}

func (s *ovnMdServer) nsRun_(ctx context.Context, f func(ctx context.Context) error) error {
	var (
		wg  = &sync.WaitGroup{}
		err error
	)
	origNs, err := netns.Get()
	if err != nil {
		return errors.Wrap(err, "get current net ns")
	}
	defer origNs.Close()
	wg.Add(1)
	go func() {
		defer wg.Done()

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		defer netns.Set(origNs)

		err = f(ctx)

	}()
	wg.Wait()
	return err
}

func (s *ovnMdServer) serveRequest(ctx context.Context) {
	type mdfData struct {
		ctx    context.Context
		cancel context.CancelFunc
	}
	var (
		mdfs   = map[*ovnMdForward]mdfData{}
		mdfsMu = &sync.Mutex{}
	)
	for {
		select {
		case req := <-s.requestC:
			switch req.Cmd {
			case ovnMdServerReqCmdOpenForward:
				var (
					mdf                       = req.Data.(*ovnMdForward)
					forwardCtx, forwardCancel = context.WithCancel(s.forwardCtx)
					mdfData                   = mdfData{
						ctx:    forwardCtx,
						cancel: forwardCancel,
					}
				)

				mdfsMu.Lock()
				mdfs[mdf] = mdfData
				mdfsMu.Unlock()

				go func() {
					defer func() {
						mdfsMu.Lock()
						defer mdfsMu.Unlock()
						delete(mdfs, mdf)
					}()

					mdf.Serve(forwardCtx)
				}()
			case ovnMdServerReqCmdCloseForward:
				var (
					data = req.Data.(*fwdpb.CloseRequest)
				)
				func() {
					mdfsMu.Lock()
					defer mdfsMu.Unlock()

					for mdf := range mdfs {
						if mdf.Proto == data.Proto &&
							mdf.BindAddr() == data.BindAddr &&
							mdf.BindPort() == int(data.BindPort) {
							mdfs[mdf].cancel()
							delete(mdfs, mdf) // delete is idempotent
							break
						}
					}
				}()
			case ovnMdServerReqCmdListForward:
				var (
					data     = req.Data.(*fwdpb.ListByRemoteRequest)
					respMdfs []*ovnMdForward
				)
				func() {
					mdfsMu.Lock()
					defer mdfsMu.Unlock()
					for mdf := range mdfs {
						if addr := mdf.RemoteAddr; addr == data.RemoteAddr {
							var (
								portm  = data.RemotePort == 0 || int(data.RemotePort) == mdf.RemotePort
								protom = data.Proto == "" || data.Proto == mdf.Proto
							)
							if portm && protom {
								respMdfs = append(respMdfs, mdf)
							}
						}
					}
				}()
				select {
				case req.RespC <- respMdfs:
				case <-ctx.Done():
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *ovnMdServer) Request(ctx context.Context, req *ovnMdServerReq) error {
	select {
	case s.requestC <- req:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
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

type nsRunFunc func(ctx context.Context, f func(ctx context.Context) error) error
type nsRunner interface {
	nsRun(ctx context.Context, f func(ctx context.Context) error) error
}

type ovnMdForward struct {
	Proto      string
	LocalAddr  string
	LocalPort  int
	RemoteAddr string
	RemotePort int

	localIP        net.IP
	remoteIP       net.IP
	listener       net.Listener
	remoteNsRunner nsRunner

	bindAddr string
	bindPort int
}

func (mdf *ovnMdForward) init() error {
	mdf.localIP = net.ParseIP(mdf.LocalAddr)
	if mdf.localIP == nil {
		return errors.Errorf("bad local address %q", mdf.LocalAddr)
	}

	mdf.remoteIP = net.ParseIP(mdf.RemoteAddr)
	if mdf.remoteIP == nil {
		return errors.Errorf("bad remote address %q", mdf.RemoteAddr)
	}

	return nil
}

func (mdf *ovnMdForward) NsRunner(runner nsRunner) *ovnMdForward {
	mdf.remoteNsRunner = runner
	return mdf
}

func (mdf *ovnMdForward) BindAddr() string {
	return mdf.bindAddr
}

func (mdf *ovnMdForward) BindPort() int {
	return mdf.bindPort
}

func (mdf *ovnMdForward) Listen() error {
	if err := mdf.init(); err != nil {
		return err
	}
	switch mdf.Proto {
	case "tcp":
		l, err := net.ListenTCP("tcp", &net.TCPAddr{
			IP:   mdf.localIP,
			Port: mdf.LocalPort,
		})
		if err != nil {
			return errors.Errorf("%s listen: %v", mdf.Proto, err)
		}
		addr := l.Addr().(*net.TCPAddr)
		mdf.bindAddr = addr.IP.String()
		mdf.bindPort = addr.Port
		mdf.listener = l
		return err
	}
	return errors.Errorf("unknown protocol: %s", mdf.Proto)
}

func (mdf *ovnMdForward) dial() (net.Conn, error) {
	switch mdf.Proto {
	case "tcp":
		conn, err := net.DialTCP("tcp", nil, &net.TCPAddr{
			IP:   mdf.remoteIP,
			Port: mdf.RemotePort,
		})
		if err != nil {
			return nil, errors.Errorf("%s dial: %v", mdf.Proto, err)
		}
		return conn, nil
	}
	return nil, errors.Errorf("unknown protocol: %s", mdf.Proto)
}

func (mdf *ovnMdForward) Serve(ctx context.Context) error {
	const idleD = 24 * time.Hour
	notifyC := make(chan utils.Empty)
	go func() {
		idleT := time.NewTimer(idleD)
		for {
			select {
			case <-ctx.Done():
				mdf.listener.Close()
				return
			case <-idleT.C:
				log.Infof("Serve idle timeout %s:%d -> %s:%d", mdf.BindAddr(), mdf.BindPort(), mdf.RemoteAddr, mdf.RemotePort)
				mdf.listener.Close()
				return
			case <-notifyC:
				idleT.Reset(idleD)
			}
		}
	}()
	for {
		conn, err := mdf.listener.Accept()
		if err != nil {
			return errors.Wrap(err, "accept")
		}
		select {
		case notifyC <- utils.Empty{}:
		default:
		}
		go func() {
			err := mdf.serveConn(ctx, conn)
			if err != nil {
				var (
					laddr = conn.LocalAddr()
					raddr = conn.RemoteAddr()
				)
				log.Errorf("serveConn %s %s->%s fail: %v", laddr.Network(), raddr, laddr, err)
			}
		}()
	}
}

func (mdf *ovnMdForward) remoteNsRun(ctx context.Context, f func(ctx context.Context) error) error {
	return mdf.remoteNsRunner.nsRun(ctx, f)
}

func (mdf *ovnMdForward) localNsRun(ctx context.Context, f func(ctx context.Context) error) error {
	return f(ctx)
}

func (mdf *ovnMdForward) serveConn(ctx context.Context, lconn net.Conn) error {
	var rconn net.Conn
	if err := mdf.remoteNsRun(ctx, func(ctx context.Context) error {
		conn, err := mdf.dial()
		if err != nil {
			return err
		}
		rconn = conn
		return nil
	}); err != nil {
		lconn.Close()
		return errors.Wrap(err, "dial remote")
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	go func() {
		select {
		case <-ctx.Done():
			lconn.Close()
			rconn.Close()
		}
	}()
	rwfunc := func(nsf nsRunFunc, conn net.Conn, dataout chan<- []byte, datain <-chan []byte) error {
		return nsf(ctx, func(ctx context.Context) error {
			const rwtimeout = time.Hour
			var (
				err0 error
				err1 error
			)

			defer conn.Close()

			wg := &sync.WaitGroup{}
			wg.Add(1)
			go func() {
				err0 = nsf(ctx, func(ctx context.Context) error {
					defer wg.Done()
					defer cancelFunc()
					for {
						buf := make([]byte, 4096)
						conn.SetReadDeadline(time.Now().Add(rwtimeout))
						n, err := conn.Read(buf)
						if n > 0 {
							select {
							case dataout <- buf[:n]:
							case <-ctx.Done():
								return nil
							}
						} else if err != nil {
							return errors.Wrap(err, "read")
						}
					}
				})
			}()

			wg.Add(1)
			go func() {
				err1 = nsf(ctx, func(ctx context.Context) error {
					defer wg.Done()
					defer cancelFunc()
					var data []byte
					for {
						select {
						case data = <-datain:
						case <-ctx.Done():
							return nil
						}
						conn.SetWriteDeadline(time.Now().Add(rwtimeout))
						n, err := conn.Write(data)
						if n != len(data) {
							return errors.Errorf("wrote %d bytes, written %d", len(data), n)
						} else if err != nil {
							return errors.Wrap(err, "write")
						}
					}
				})
			}()

			wg.Wait()

			err := errors.NewAggregate([]error{err0, err1})
			if err != nil {
				var (
					laddr = conn.LocalAddr()
					raddr = conn.RemoteAddr()
				)
				return errors.Wrapf(err, "conn %s %s->%s", laddr.Network(), raddr, laddr)
			}
			return nil
		})
	}
	var (
		datacslr   = make(chan []byte) // source: local read
		datacsrr   = make(chan []byte) // source: remote read
		wg         = &sync.WaitGroup{}
		err0, err1 error
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		err0 = rwfunc(mdf.localNsRun, lconn, datacslr, datacsrr)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		// mdf.localNsRun is not typo
		err1 = rwfunc(mdf.localNsRun, rconn, datacsrr, datacslr)
	}()
	wg.Wait()

	return errors.NewAggregate([]error{err0, err1})
}

type netIdIP struct {
	netId   string
	ip      string
	masklen int

	ip6      string
	masklen6 int

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

			ip6:      nic.IP6,
			masklen6: nic.Masklen6,
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

	grpcFwdReqC chan ovnMdFwdReq
}

func newOvnMdMan(watcher *serversWatcher) *ovnMdMan {
	man := &ovnMdMan{
		watcher: watcher,
		c:       make(chan *ovnMdReq),

		mdServers: map[string]*ovnMdServer{},
		guestNics: map[string]netIdIPm{},

		grpcFwdReqC: make(chan ovnMdFwdReq),
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
		case mdreq := <-man.grpcFwdReqC:
			man.handleMdPbReq(ctx, mdreq)
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
		var netCidr, netCidr6 string
		if len(nii.ip) > 0 {
			ipmask := fmt.Sprintf("%s/%d", nii.ip, nii.masklen)
			_, ipnet, err := net.ParseCIDR(ipmask)
			if err != nil {
				log.Errorf("guestId %s, ip %s, parse cidr: %s", nii.guestId, nii.ip, ipmask)
			}
			netCidr = ipnet.String()
		}
		if len(nii.ip6) > 0 {
			ipmask := fmt.Sprintf("%s/%d", nii.ip6, nii.masklen6)
			_, ipnet, err := net.ParseCIDR(ipmask)
			if err != nil {
				log.Errorf("guestId %s, ip6 %s, parse cidr: %s", nii.guestId, nii.ip6, ipmask)
			}
			netCidr6 = ipnet.String()
		}
		mdServer = newOvnMdServer(netId, netCidr, netCidr6, man.watcher)
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

func (man *ovnMdMan) ForwardRequest(ctx context.Context, mdreq ovnMdFwdReq) (protoreflect.ProtoMessage, error) {
	return mdreq.Do(ctx, man.grpcFwdReqC)
}

func (man *ovnMdMan) handleMdPbReq(ctx context.Context, mdreq ovnMdFwdReq) {
	switch pbreq := mdreq.pbreq.(type) {
	case *fwdpb.OpenRequest:
		netId := pbreq.NetId
		mdServer, ok := man.mdServers[netId]
		if !ok {
			mdreq.RespErr(ctx, errors.Errorf("forward not available for netId %s", netId))
			return
		}
		mdf := &ovnMdForward{
			Proto:      pbreq.Proto,
			LocalAddr:  pbreq.BindAddr,
			LocalPort:  int(pbreq.BindPort),
			RemoteAddr: pbreq.RemoteAddr,
			RemotePort: int(pbreq.RemotePort),
		}
		mdf = mdf.NsRunner(mdServer)
		if err := mdf.Listen(); err != nil {
			mdreq.RespErr(ctx, errors.Wrap(err, "listen"))
			return
		}
		if err := mdServer.Request(ctx, &ovnMdServerReq{
			Cmd:  ovnMdServerReqCmdOpenForward,
			Data: mdf,
		}); err != nil {
			mdreq.RespErr(ctx, errors.Wrap(err, "forward"))
			return
		}
		log.Infof("forward net %s, %s local %s:%d, remote %s:%d",
			netId,
			mdf.Proto,
			mdf.BindAddr(),
			mdf.BindPort(),
			mdf.RemoteAddr,
			mdf.RemotePort,
		)
		pbresp := &fwdpb.OpenResponse{
			NetId:      netId,
			Proto:      mdf.Proto,
			BindAddr:   mdf.BindAddr(),
			BindPort:   uint32(mdf.BindPort()),
			RemoteAddr: mdf.RemoteAddr,
			RemotePort: uint32(mdf.RemotePort),
		}
		mdreq.RespPb(ctx, pbresp)
	case *fwdpb.CloseRequest:
		var (
			netId = pbreq.NetId
		)
		mdServer, ok := man.mdServers[netId]
		if !ok {
			mdreq.RespErr(ctx, errors.Errorf("forward not available for netId %s", netId))
			return
		}
		if err := mdServer.Request(ctx, &ovnMdServerReq{
			Cmd:  ovnMdServerReqCmdCloseForward,
			Data: pbreq,
		}); err != nil {
			mdreq.RespErr(ctx, err)
			return
		}
		pbresp := &fwdpb.CloseResponse{
			NetId:    netId,
			Proto:    pbreq.Proto,
			BindAddr: pbreq.BindAddr,
			BindPort: pbreq.BindPort,
		}
		mdreq.RespPb(ctx, pbresp)
	case *fwdpb.ListByRemoteRequest:
		var (
			netId  = pbreq.NetId
			pbresp = &fwdpb.ListByRemoteResponse{}
		)
		mdServer, ok := man.mdServers[netId]
		if !ok {
			mdreq.RespPb(ctx, pbresp)
			return
		}
		var (
			respC = make(chan interface{})
			mdfs  []*ovnMdForward
		)
		if err := mdServer.Request(ctx, &ovnMdServerReq{
			Cmd:   ovnMdServerReqCmdListForward,
			Data:  pbreq,
			RespC: respC,
		}); err != nil {
			mdreq.RespErr(ctx, err)
			return
		}
		select {
		case resp := <-respC:
			mdfs = resp.([]*ovnMdForward)
		case <-ctx.Done():
			mdreq.RespErr(ctx, ctx.Err())
			return
		}
		for _, mdf := range mdfs {
			pbresp.Forwards = append(pbresp.Forwards, &fwdpb.OpenResponse{
				Proto:      mdf.Proto,
				BindAddr:   mdf.BindAddr(),
				BindPort:   uint32(mdf.BindPort()),
				RemoteAddr: mdf.RemoteAddr,
				RemotePort: uint32(mdf.RemotePort),
			})
		}
		mdreq.RespPb(ctx, pbresp)
	default:
		mdreq.RespErr(ctx, errors.Errorf("unknown pb message: %t", pbreq))
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
