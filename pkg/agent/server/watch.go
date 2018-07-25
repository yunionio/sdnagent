package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"path"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/digitalocean/go-openvswitch/ovs"
	"github.com/fsnotify/fsnotify"

	"yunion.io/yunion-sdnagent/pkg/agent/utils"
	"yunion.io/yunioncloud/pkg/log"
)

var REGEX_UUID *regexp.Regexp = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

type serversWatcher struct {
	hostConfig *utils.HostConfig
	watcher    *fsnotify.Watcher
	guests     map[string]*utils.Guest
	agent      *AgentServer
	zoneMan    *utils.ZoneMan
	ofCli      *ovs.OpenFlowService
}

func newServersWatcher() (*serversWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	hc, err := utils.NewHostConfig(DefaultHostConfigPath)
	if err != nil {
		return nil, err
	}
	return &serversWatcher{
		hostConfig: hc,
		watcher:    w,
		guests:     map[string]*utils.Guest{},
		zoneMan:    utils.NewZoneMan(GuestCtZoneBase),
		ofCli:      ovs.New().OpenFlow,
	}, nil
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

func (w *serversWatcher) reloadGuestDesc(ctx context.Context, g *utils.Guest) error {
	oldM := map[string]uint16{}
	oldNICs := g.NICs
	for _, nic := range oldNICs {
		oldM[nic.MAC] = nic.CtZoneId
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
		zoneId, err := w.zoneMan.AllocateZoneId(nic.MAC)
		if err != nil {
			return fmt.Errorf("ct zone id allocation failed: %s", err)
		}
		nic.CtZoneId = zoneId
	}
	for mac, _ := range oldM {
		w.zoneMan.FreeZoneId(mac)
	}
	return nil
}

func (w *serversWatcher) guestReady(ctx context.Context, g *utils.Guest) bool {
	for _, nic := range g.NICs {
		bridge := nic.Bridge
		ifname := nic.IfnameHost
		portStats, err := w.ofCli.DumpPort(bridge, ifname)
		if err != nil {
			return false
		}
		nic.PortNo = portStats.PortID
	}
	return true
}

func (w *serversWatcher) updGuestFlows(ctx context.Context, g *utils.Guest) {
	// TODO TODO tick faster on error
	err := w.reloadGuestDesc(ctx, g)
	if err != nil {
		log.Warningf("guest %s load desc failed: %s", g.Id, err)
		return
	}
	if !w.guestReady(ctx, g) {
		log.Warningf("guest %s is not ready", g.Id)
		return
	}
	bfs, err := g.FlowsMap()
	if err != nil {
	}
	for bridge, flows := range bfs {
		flowman := w.agent.GetFlowMan(bridge)
		flowman.updateFlows(ctx, g.Who(), flows)
	}
}

func (w *serversWatcher) delGuestFlows(ctx context.Context, g *utils.Guest) {
	if g.NICs == nil {
		return
	}
	bridges := map[string]bool{}
	for _, nic := range g.NICs {
		bridges[nic.Bridge] = true
		w.zoneMan.FreeZoneId(nic.MAC)
	}
	for bridge, _ := range bridges {
		flowman := w.agent.GetFlowMan(bridge)
		flowman.updateFlows(ctx, g.Who(), []*ovs.Flow{})
	}
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
		name := fi.Name()
		if REGEX_UUID.MatchString(name) {
			path := path.Join(serversPath, name)
			g := &utils.Guest{
				Id:         name,
				Path:       path,
				HostConfig: w.hostConfig,
			}
			err := w.watcher.Add(path)
			if err != nil {
				log.Errorf("watch guest path %s failed during scan: %s", path, err)
			}
			w.guests[name] = g
			w.updGuestFlows(ctx, g)
		}
	}
}

func (w *serversWatcher) updHostLocalFlows(ctx context.Context) {
	for _, hcn := range w.hostConfig.Networks {
		ip, err := w.hostConfig.MasterIP()
		if err != nil {
			log.Errorf("get master ip failed; %s", err)
			continue
		}
		mac, err := w.hostConfig.MasterMAC()
		if err != nil {
			log.Errorf("get master mac failed; %s", err)
			continue
		}
		hostLocal := &utils.HostLocal{
			MetadataPort: w.hostConfig.Port,
			K8SCidr:      w.hostConfig.K8sClusterCidr,
			Bridge:       hcn.Bridge,
			Ifname:       hcn.Ifname,
			IP:           ip,
			MAC:          mac,
		}
		flows, err := hostLocal.FlowsMap()
		if err != nil {
			log.Errorf("prepare %s hostlocal flows failed: %s", hcn.Bridge, err)
			continue
		}
		flowman := w.agent.GetFlowMan(hcn.Bridge)
		flowman.updateFlows(ctx, hostLocal.Who(), flows[hcn.Bridge])
	}
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

func (w *serversWatcher) Start(ctx context.Context) {
	wg := ctx.Value("wg").(*sync.WaitGroup)
	wg.Add(1)
	defer wg.Done()
	defer w.watcher.Close()
	err := w.watcher.Add(w.hostConfig.ServersPath)
	if err != nil {
		return
	}
	w.withWait(ctx, func(ctx context.Context) {
		w.updHostLocalFlows(ctx)
		w.scan(ctx)
	})
	refreshTicker := time.NewTicker(WatcherRefreshRate)
	defer refreshTicker.Stop()
	for {
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
				log.Errorf("received guest path add event: %s", guestPath)
				err := w.watcher.Add(guestPath)
				if err != nil {
					log.Errorf("watch guest path %s failed: %s", guestPath, err)
				}
				g := &utils.Guest{
					Id:         guestId,
					Path:       guestPath,
					HostConfig: w.hostConfig,
				}
				w.guests[guestId] = g
				w.updGuestFlows(ctx, g)
			case watchEventTypeDelServerDir:
				if g, ok := w.guests[guestId]; ok {
					// this is needed for containers
					w.delGuestFlows(ctx, g)
					delete(w.guests, guestId)
				}
				log.Infof("guest path deleted: %s", guestPath)
			case watchEventTypeUpdServer:
				if g, ok := w.guests[guestId]; ok {
					w.updGuestFlows(ctx, g)
				} else {
					log.Warningf("unexpected guest update event: %s", guestPath)
				}
			case watchEventTypeDelServer:
				if g, ok := w.guests[guestId]; ok {
					log.Infof("remove guest flows %s", guestId)
					w.delGuestFlows(ctx, g)
				} else {
					log.Warningf("unexpected guest down event: %s", guestPath)
				}
			}
		case <-refreshTicker.C:
			log.Infof("watcher refresh time ;)")
			w.withWait(ctx, func(ctx context.Context) {
				w.updHostLocalFlows(ctx)
				for _, g := range w.guests {
					w.updGuestFlows(ctx, g)
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
