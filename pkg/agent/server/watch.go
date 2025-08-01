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
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"yunion.io/x/log"

	"yunion.io/x/onecloud/pkg/hostman/guestman/desc"
	fwdpb "yunion.io/x/onecloud/pkg/hostman/guestman/forwarder/api"

	"yunion.io/x/sdnagent/pkg/agent/utils"
)

var REGEX_UUID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

/*type pendingGuest struct {
	guest     *utils.Guest
	firstSeen time.Time
}*/

type wCmd int

const (
	wCmdFindGuestDescByIdIP wCmd = iota
	wCmdFindGuestDescByHostLocalIP
)

type wCmdFindGuestDescByIdIPData struct {
	NetId  string
	IP     string
	RespCh chan<- *desc.SGuestDesc
}

type wCmdFindGuestDescByHostLocalIPData struct {
	HostLocal *utils.HostLocal
	IP        string
	RespCh    chan<- *desc.SGuestDesc
}

type wCmdReq struct {
	cmd  wCmd
	data interface{}
}

type serversWatcher struct {
	agent    *AgentServer
	tcMan    *TcMan
	ovnMan   *ovnMan
	ovnMdMan *ovnMdMan

	hostConfig *utils.HostConfig
	watcher    *fsnotify.Watcher
	hostLocal  *HostLocal
	guests     map[string]*Guest
	zoneMan    *utils.ZoneMan

	cmdCh chan wCmdReq

	bridgeIpNicCache map[string]*desc.SGuestDesc
	netIdIpNicCache  map[string]*desc.SGuestDesc
}

func newServersWatcher() (*serversWatcher, error) {
	w := &serversWatcher{
		guests:  map[string]*Guest{},
		zoneMan: utils.NewZoneMan(GuestCtZoneBase),

		cmdCh: make(chan wCmdReq),

		bridgeIpNicCache: make(map[string]*desc.SGuestDesc),
		netIdIpNicCache:  make(map[string]*desc.SGuestDesc),
	}
	return w, nil
}

func (w *serversWatcher) newForwardService() fwdpb.ForwarderServer {
	w.hostConfig = w.agent.hostConfig
	if !w.hostConfig.DisableLocalVpc {
		w.ovnMan = newOvnMan(w)
		w.ovnMdMan = newOvnMdMan(w)
	}
	return newOvnMdFwdService(w.ovnMdMan)
}

type watchEventType int

const (
	watchEventTypeAddServerDir watchEventType = iota
	watchEventTypeDelServerDir
	watchEventTypeUpdServer
	watchEventTypeDelServer
)

var watchEventTypeStringMap = []string{
	"watchEventTypeAddServerDir",
	"watchEventTypeDelServerDir",
	"watchEventTypeUpdServer",
	"watchEventTypeDelServer",
}

type watchEvent struct {
	evType    watchEventType
	guestId   string
	guestPath string // path to the servers/<uuid> dir
}

func (w *watchEvent) String() string {
	return fmt.Sprintf("type: %s guest_id: %s path: %s", watchEventTypeStringMap[w.evType], w.guestId, w.guestPath)
}

func (w *serversWatcher) scan(ctx context.Context) {
	serversPath := w.hostConfig.ServersPath
	fis, err := os.ReadDir(serversPath)
	if err != nil {
		log.Errorf("scan servers path %s failed: %s", serversPath, err)
		return
	}
	for _, fi := range fis {
		if !fi.IsDir() {
			continue
		}
		id := fi.Name()
		if REGEX_UUID.MatchString(id) {
			guestStart := time.Now()
			log.Infof("scan guest %s", id)
			path := path.Join(serversPath, id)
			g, err := w.addGuestWatch(id, path)
			if err != nil {
				log.Errorf("inotify events watch guest failed during scan: %s: %s", path, err)
			}
			log.Infof("end of scan guest %s addGuestWatch: %f", id, time.Since(guestStart).Seconds())
			g.UpdateSettings(ctx, false)
			log.Infof("end of scan guest %s: %f", id, time.Since(guestStart).Seconds())
		}
	}
}

// addGuestWatch adds the server with <id> in <path> to watch list.  It returns
// error when adding watch failed, but it will always return non-nil *Guest
func (w *serversWatcher) addGuestWatch(id, path string) (*Guest, error) {
	if g, ok := w.guests[id]; ok {
		return g, nil
	}
	ug := &utils.Guest{
		Id:         id,
		Path:       path,
		HostConfig: w.hostConfig,
	}
	g := NewGuest(ug, w)
	w.guests[id] = g
	err := w.watcher.Add(path)
	return g, err
}

func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

func (w *serversWatcher) withWait(ctx context.Context, f func(context.Context)) {
	waitData := map[string]*FlowManWaitData{}
	ctx = context.WithValue(ctx, "waitData", waitData)
	start := time.Now()
	funcName := GetFunctionName(f)
	log.Debugf("[serversWatcher] start wait %s context ....", funcName)
	f(ctx)
	log.Debugf("[serversWatcher] end wait %s context %f....", funcName, time.Since(start).Seconds())
	for _, wd := range waitData {
		wd.FlowMan.waitDecr(wd.Count)
		wd.FlowMan.SyncFlows(ctx)
	}
	w.tcMan.SyncAll(ctx)
}

func (w *serversWatcher) hasRecentPending() bool {
	for _, g := range w.guests {
		if g.IsPending() {
			return true
		}
	}
	return false
}

func (w *serversWatcher) Start(ctx context.Context, agent *AgentServer) {
	defer agent.Stop()

	// workgroup
	wg := ctx.Value("wg").(*sync.WaitGroup)
	defer wg.Done()

	w.agent = agent

	var err error

	// start watcher before scan
	w.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Errorf("creating inotify events watcher failed: %s", err)
		return
	}
	defer w.watcher.Close()
	err = w.watcher.Add(w.hostConfig.ServersPath)
	if err != nil {
		log.Errorf("watching %s failed: %s", w.hostConfig.ServersPath, err)
		return
	}

	if w.hostConfig.SdnEnableTcMan {
		w.tcMan = NewTcMan()
		wg.Add(1)
		go w.tcMan.Start(ctx)
	}

	if !w.hostConfig.DisableLocalVpc {
		wg.Add(1)
		go w.ovnMan.Start(ctx)

		wg.Add(1)
		go w.ovnMdMan.Start(ctx)
	}

	// init scan
	w.hostLocal = NewHostLocal(w)
	w.withWait(ctx, func(ctx context.Context) {
		w.hostLocal.UpdateSettings(ctx, false)
		w.scan(ctx)
		log.Infof("serversWatcher.Start: Finish initial guests scan")
	})

	refreshTicker := time.NewTicker(WatcherRefreshRate)
	pendingRefreshTicker := time.NewTicker(WatcherRefreshRateOnError)
	defer refreshTicker.Stop()
	defer pendingRefreshTicker.Stop()
	for {
		var pendingChan <-chan time.Time
		if w.hasRecentPending() {
			pendingChan = pendingRefreshTicker.C
		}
		select {
		case ev, ok := <-w.watcher.Events:
			if !ok {
				log.Errorf("fsnotity.watch.Events error")
				goto out
			}
			log.Infof("receive inotify events!")
			wev := w.watchEvent(&ev)
			if wev == nil {
				log.Debugf("inotify events ignored: %s", ev)
			} else {
				log.Debugf("to handle inotify events %s %s", ev, wev)
				guestId := wev.guestId
				guestPath := wev.guestPath
				switch wev.evType {
				case watchEventTypeAddServerDir:
					log.Infof("received guest path add event: %s", guestPath)
					g, err := w.addGuestWatch(guestId, guestPath)
					if err != nil {
						log.Errorf("watch guest failed: %s: %s", guestPath, err)
					}
					g.UpdateSettings(ctx, true)
				case watchEventTypeDelServerDir:
					if g, ok := w.guests[guestId]; ok {
						// this is needed for containers
						g.ClearSettings(ctx)
						delete(w.guests, guestId)
					}
					log.Infof("guest path deleted: %s", guestPath)
				case watchEventTypeUpdServer:
					log.Infof("watchEventTypeUpdServer %s", guestId)
					if g, ok := w.guests[guestId]; ok {
						g.UpdateSettings(ctx, true)
					} else {
						log.Warningf("unexpected guest update event: %s", guestPath)
					}
				case watchEventTypeDelServer:
					if g, ok := w.guests[guestId]; ok {
						log.Infof("remove guest settings %s", guestId)
						g.ClearSettings(ctx)
					} else {
						log.Warningf("unexpected guest down event: %s", guestPath)
					}
				}
			}
		case <-pendingChan:
			log.Infof("watcher refresh pendings")
			w.withWait(ctx, func(ctx context.Context) {
				for _, g := range w.guests {
					if g.IsPending() {
						g.UpdateSettings(ctx, false)
					}
				}
			})
		case <-refreshTicker.C:
			log.Infof("watcher refresh time ;)")
			w.withWait(ctx, func(ctx context.Context) {
				w.hostLocal.UpdateSettings(ctx, false)
				w.scan(ctx)
				// for _, g := range w.guests {
				//	g.UpdateSettings(ctx)
				// }
			})
		case err, ok := <-w.watcher.Errors:
			if !ok {
				log.Errorf("fsnotity.watch.Errors error")
				goto out
			}
			// fail fast and recover fresh
			panic("watcher error: %s" + err.Error())
			return
		case cmd := <-w.cmdCh:
			switch cmd.cmd {
			case wCmdFindGuestDescByIdIP:
				var (
					data  = cmd.data.(wCmdFindGuestDescByIdIPData)
					netId = data.NetId
					ip    = data.IP
					robj  *desc.SGuestDesc
				)
				if gDesc, ok := w.netIdIpNicCache[netId]; ok {
					robj = gDesc
				} else {
					for guestId, guest := range w.guests {
						if nic := guest.FindNicByNetIdIP(netId, ip); nic != nil {
							obj, err := guest.GetJSONObjectDesc()
							if err != nil {
								log.Errorf("guest %s: GetJSONObjectDesc: %v", guestId, err)
							}
							robj = obj
							break
						}
					}
					if robj != nil {
						w.netIdIpNicCache[netId] = robj
					}
				}
				data.RespCh <- robj
			case wCmdFindGuestDescByHostLocalIP:
				var (
					data      = cmd.data.(wCmdFindGuestDescByHostLocalIPData)
					hostLocal = data.HostLocal
					ip        = data.IP
					robj      *desc.SGuestDesc
				)
				mapKey := fmt.Sprintf("%s:%s", hostLocal.Bridge, ip)
				if gDesc, ok := w.bridgeIpNicCache[mapKey]; ok {
					robj = gDesc
				} else {
					for guestId, guest := range w.guests {
						if nic := guest.FindNicByHostLocalIP(hostLocal, ip); nic != nil {
							obj, err := guest.GetJSONObjectDesc()
							if err != nil {
								log.Errorf("guest %s: GetJSONObjectDesc: %v", guestId, err)
							}
							robj = obj
							break
						}
					}
					if robj != nil {
						w.bridgeIpNicCache[mapKey] = robj
					}
				}
				data.RespCh <- robj
			}
		case <-ctx.Done():
			log.Infof("watcher bye")
			goto out
		}
	}
out:
}

func (w *serversWatcher) FindGuestDescByNetIdIP(netId, ip string) *desc.SGuestDesc {
	respCh := make(chan *desc.SGuestDesc)
	reqData := wCmdFindGuestDescByIdIPData{
		NetId:  netId,
		IP:     ip,
		RespCh: respCh,
	}
	req := wCmdReq{
		cmd:  wCmdFindGuestDescByIdIP,
		data: reqData,
	}
	w.cmdCh <- req
	obj := <-respCh
	return obj
}

func (w *serversWatcher) FindGuestDescByHostLocalIp(hostLocal *utils.HostLocal, ip string) *desc.SGuestDesc {
	respCh := make(chan *desc.SGuestDesc)
	reqData := wCmdFindGuestDescByHostLocalIPData{
		HostLocal: hostLocal,
		IP:        ip,
		RespCh:    respCh,
	}
	req := wCmdReq{
		cmd:  wCmdFindGuestDescByHostLocalIP,
		data: reqData,
	}
	w.cmdCh <- req
	obj := <-respCh
	return obj
}

func (w *serversWatcher) watchEvent(ev *fsnotify.Event) (wev *watchEvent) {
	dir, file := filepath.Split(ev.Name)
	dir = path.Clean(dir)
	if REGEX_UUID.MatchString(file) && dir == w.hostConfig.ServersPath {
		wev = &watchEvent{
			guestId:   file,
			guestPath: ev.Name,
		}
		if ev.Op&fsnotify.Create != 0 {
			wev.evType = watchEventTypeAddServerDir
			return wev
		} else if ev.Op&fsnotify.Remove != 0 {
			wev.evType = watchEventTypeDelServerDir
			return wev
		}
	} else if file == "desc" {
		_, guestId := filepath.Split(dir)
		if ev.Op&fsnotify.Write != 0 {
			wev = &watchEvent{
				evType:    watchEventTypeUpdServer,
				guestId:   guestId,
				guestPath: dir,
			}
			return wev
		}
	} else if file == "pid" {
		_, guestId := filepath.Split(dir)
		wev = &watchEvent{
			guestId:   guestId,
			guestPath: dir,
		}
		if ev.Op&fsnotify.Remove != 0 {
			wev.evType = watchEventTypeDelServer
			return wev
		} else if ev.Op&fsnotify.Write != 0 {
			wev.evType = watchEventTypeUpdServer
			return wev
		}
	}
	return nil
}

func (w *serversWatcher) GetHostLocalByIp(ip string) *utils.HostLocal {
	for _, hl := range w.hostLocal.bridgeMap {
		if hl.IP.String() == ip {
			return hl
		}
	}
	return nil
}
