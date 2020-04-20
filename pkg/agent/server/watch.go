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
	"io/ioutil"
	"path"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"yunion.io/x/log"
	"yunion.io/x/sdnagent/pkg/agent/utils"
)

var REGEX_UUID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

type pendingGuest struct {
	guest     *utils.Guest
	firstSeen time.Time
}

type serversWatcher struct {
	agent      *AgentServer
	tcMan      *TcMan
	ovnMan     *ovnMan
	hostConfig *utils.HostConfig
	watcher    *fsnotify.Watcher
	hostLocal  *HostLocal
	guests     map[string]*Guest
	zoneMan    *utils.ZoneMan
}

func newServersWatcher() (*serversWatcher, error) {
	w := &serversWatcher{
		guests:  map[string]*Guest{},
		zoneMan: utils.NewZoneMan(GuestCtZoneBase),
		tcMan:   NewTcMan(),
	}
	w.ovnMan = newOvnMan(w)
	return w, nil
}

type watchEventType int

const (
	watchEventTypeAddServerDir watchEventType = iota
	watchEventTypeDelServerDir
	watchEventTypeUpdServer
	watchEventTypeDelServer
)

type watchEvent struct {
	evType    watchEventType
	guestId   string
	guestPath string // path to the servers/<uuid> dir
}

func (w *serversWatcher) scan(ctx context.Context) {
	serversPath := w.hostConfig.ServersPath
	fis, err := ioutil.ReadDir(serversPath)
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
			path := path.Join(serversPath, id)
			g, err := w.addGuestWatch(id, path)
			if err != nil {
				log.Errorf("watch guest failed during scan: %s: %s", path, err)
			}
			g.UpdateSettings(ctx)
		}
	}
}

// addGuestWatch adds the server with <id> in <path> to watch list.  It returns
// error when adding watch failed, but it will always return non-nil *Guest
func (w *serversWatcher) addGuestWatch(id, path string) (*Guest, error) {
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

func (w *serversWatcher) withWait(ctx context.Context, f func(context.Context)) {
	waitData := map[string]*FlowManWaitData{}
	ctx = context.WithValue(ctx, "waitData", waitData)
	f(ctx)
	for _, wd := range waitData {
		wd.FlowMan.waitDecr(ctx, wd.Count)
		wd.FlowMan.SyncFlows(ctx)
	}
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

	// hostConfig
	hc, err := utils.NewHostConfig(DefaultHostConfigPath)
	if err != nil {
		log.Errorf("getting host config failed: %s", err)
		return
	}
	w.hostConfig = hc
	if err := w.hostConfig.Auth(ctx); err != nil {
		log.Errorf("keystone auth: %v", err)
		return
	}

	ctx, cancelFunc := context.WithCancel(ctx)
	go w.hostConfig.WatchMtimeChange(ctx, func(mtime time.Time) {
		log.Warningf("%s mtime changed, now %s", DefaultHostConfigPath, mtime)
		cancelFunc()
	})

	// start watcher before scan
	w.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Errorf("creating inotify watcher failed: %s", err)
		return
	}
	defer w.watcher.Close()
	err = w.watcher.Add(w.hostConfig.ServersPath)
	if err != nil {
		log.Errorf("watching %s failed: %s", w.hostConfig.ServersPath, err)
		return
	}

	wg.Add(1)
	go w.tcMan.Start(ctx)

	wg.Add(1)
	go w.ovnMan.Start(ctx)

	// init scan
	w.hostLocal = NewHostLocal(w)
	w.withWait(ctx, func(ctx context.Context) {
		w.hostLocal.UpdateSettings(ctx)
		w.scan(ctx)
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
		case ev := <-w.watcher.Events:
			wev := w.watchEvent(&ev)
			if wev == nil {
				log.Debugf("inotify event ignored: %s", ev)
				break
			}
			guestId := wev.guestId
			guestPath := wev.guestPath
			switch wev.evType {
			case watchEventTypeAddServerDir:
				log.Infof("received guest path add event: %s", guestPath)
				g, err := w.addGuestWatch(guestId, guestPath)
				if err != nil {
					log.Errorf("watch guest failed: %s: %s", guestPath, err)
				}
				g.UpdateSettings(ctx)
			case watchEventTypeDelServerDir:
				if g, ok := w.guests[guestId]; ok {
					// this is needed for containers
					g.ClearSettings(ctx)
					delete(w.guests, guestId)
				}
				log.Infof("guest path deleted: %s", guestPath)
			case watchEventTypeUpdServer:
				if g, ok := w.guests[guestId]; ok {
					g.UpdateSettings(ctx)
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
		case <-pendingChan:
			log.Infof("watcher refresh pendings")
			w.withWait(ctx, func(ctx context.Context) {
				for _, g := range w.guests {
					if g.IsPending() {
						g.UpdateSettings(ctx)
					}
				}
			})
		case <-refreshTicker.C:
			log.Infof("watcher refresh time ;)")
			w.withWait(ctx, func(ctx context.Context) {
				w.hostLocal.UpdateSettings(ctx)
				for _, g := range w.guests {
					g.UpdateSettings(ctx)
				}
			})
		case err := <-w.watcher.Errors:
			// fail fast and recover fresh
			panic("watcher error: %s" + err.Error())
			return
		case <-ctx.Done():
			log.Infof("watcher bye")
			goto out
		}
	}
out:
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
