package utils

import (
	"sync"
	"time"

	"github.com/digitalocean/go-openvswitch/ovs"
)

const (
	stalePeriod = 3 * time.Second
	cacheLimit  = 2048
)

var (
	portStatsCache = NewPortStatsCache()
)

func DumpPort(bridge, port string) (*ovs.PortStats, error) {
	return portStatsCache.DumpPort(bridge, port)
}

type PortStatsCache struct {
	cli *ovs.OpenFlowService

	rw    *sync.RWMutex
	store map[string]*portStatsData
}

func NewPortStatsCache() *PortStatsCache {
	cache := &PortStatsCache{
		cli:   ovs.New().OpenFlow,
		rw:    &sync.RWMutex{},
		store: map[string]*portStatsData{},
	}
	return cache
}

func (cache *PortStatsCache) cleanStales() {
	cache.rw.Lock()
	defer cache.rw.Unlock()

	for k, v := range cache.store {
		if v.staled() {
			delete(cache.store, k)
		}
	}
}

func (cache *PortStatsCache) DumpPort(bridge, port string) (*ovs.PortStats, error) {
	key := bridge + "," + port

	cache.rw.RLock()
	if len(cache.store) >= cacheLimit {
		go cache.cleanStales()
	}
	if data, ok := cache.store[key]; ok && !data.staled() {
		ps := data.get()
		cache.rw.RUnlock()
		return ps, nil
	}
	cache.rw.RUnlock()

	ps, err := cache.cli.DumpPort(bridge, port)
	if err != nil {
		return ps, err
	}

	cache.rw.Lock()
	defer cache.rw.Unlock()
	cache.store[key] = newPortStatsData(ps)
	return ps, nil
}

type portStatsData struct {
	portStats *ovs.PortStats
	created   time.Time
}

func newPortStatsData(ps *ovs.PortStats) *portStatsData {
	return &portStatsData{
		portStats: ps,
		created:   time.Now(),
	}
}

func (data *portStatsData) staled() bool {
	return time.Since(data.created) >= stalePeriod
}

func (data *portStatsData) get() *ovs.PortStats {
	return data.portStats
}
