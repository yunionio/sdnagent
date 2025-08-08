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

package utils

import (
	"crypto/sha256"
	"fmt"
	"time"

	"yunion.io/x/log"

	"yunion.io/x/onecloud/pkg/cloudcommon/app"
	common_options "yunion.io/x/onecloud/pkg/cloudcommon/options"
	"yunion.io/x/onecloud/pkg/hostman/guestman/desc"
	"yunion.io/x/onecloud/pkg/hostman/metadata"
	"yunion.io/x/onecloud/pkg/util/netutils2"
)

func (h *HostLocal) fakeMdSrcIpMac(port int) (string, string, string) {
	return fakeMdSrcIpMac(h.IP.String(), h.MAC.String(), port)
}

func fakeMdSrcIpMac(ip, mac string, port int) (string, string, string) {
	const (
		fakeMdSrcIpPrefix   = "100.%d.%d.0/20"
		fakeMdSrcIpPattern  = "100.%d.%d.%d"
		fakeMdSrcMacPattern = "ee:ee:ee:%02x:%02x:00"
	)
	hash := sha256.New()
	hash.Write([]byte(ip))
	hash.Write([]byte(mac))
	s := hash.Sum(nil)
	a := s[0]
	b := s[1] & 0xf0
	hash.Write([]byte(fmt.Sprintf("%d", port)))
	s = hash.Sum(nil)
	c := s[0] & 0x0f
	d := s[1]
	return fmt.Sprintf(fakeMdSrcIpPrefix, a, b), fmt.Sprintf(fakeMdSrcIpPattern, a, b+c, d), fmt.Sprintf(fakeMdSrcMacPattern, a, b)
}

func (h *HostLocal) fakeMdSrcIp6Mac(port int, metaSrvIp6 string) (string, string, string) {
	return fakeMdSrcIp6Mac(h.IP6Local.String(), h.MAC.String(), port, metaSrvIp6)
}

func mergeByte(a, b byte) uint16 {
	return uint16(a)<<8 | uint16(b)
}

// prefix, ip6, mac
func fakeMdSrcIp6Mac(ip6, mac string, port int, metaSrvIp6 string) (string, string, string) {
	const (
		fakeMdSrcIp6Prefix   = "fd00:fec2::%x:0/112"
		fakeMdSrcIp6Pattern  = "fd00:fec2::%x:%x"
		fakeMdSrcMac6Pattern = "ee:ee:ee:%02x:%02x:01"
	)
	hash := sha256.New()
	hash.Write([]byte(ip6))
	hash.Write([]byte(mac))
	s := hash.Sum(nil)
	a := s[0]
	b := s[1]
	hash.Write([]byte(fmt.Sprintf("%d", port)))
	hash.Write([]byte(metaSrvIp6))
	s = hash.Sum(nil)
	c := s[0]
	d := s[1]
	return fmt.Sprintf(fakeMdSrcIp6Prefix, mergeByte(a, b)), fmt.Sprintf(fakeMdSrcIp6Pattern, mergeByte(a, b), mergeByte(c, d)), fmt.Sprintf(fakeMdSrcMac6Pattern, a, b)
}

func (h *HostLocal) StartMetadataServer(watcher IServerWatcher) {
	var addr string
	sport := h.HostConfig.MetadataPort() + 1
	for port := sport; port < sport+64; port++ {
		if !netutils2.IsTcpPortUsed(addr, port) {
			h.metadataPort = port
		}
	}
	if h.metadataPort == 0 {
		log.Fatalf("Failed to find a free metadata port")
	}
	svc := &metadata.Service{
		Address: addr,
		Port:    h.metadataPort,

		DescGetter: &sClassicMetadataDescGetter{
			hostLocal: h,
			watcher:   watcher,
		},
	}
	dbAccess := false
	h.metadataApp = app.InitApp(&common_options.BaseOptions{
		ApplicationID:          "metadata-server-class-network",
		RequestWorkerCount:     4,
		RequestWorkerQueueSize: 128,
	}, dbAccess)
	log.Infof("Start metadata server at %s on port %s:%d", h.Bridge, addr, h.metadataPort)
	metadata.Start(h.metadataApp, svc)
}

type IServerWatcher interface {
	FindGuestDescByHostLocalIp(hostLocal *HostLocal, ip string) *desc.SGuestDesc
}

type sClassicMetadataDescGetter struct {
	hostLocal *HostLocal
	watcher   IServerWatcher
}

func (g *sClassicMetadataDescGetter) Get(ip string) *desc.SGuestDesc {
	start := time.Now()
	log.Infof("Get guest desc by ip %s", ip)
	guestDesc := g.watcher.FindGuestDescByHostLocalIp(g.hostLocal, ip)
	log.Infof("Get guest desc by ip %s cost %f seconds", ip, time.Since(start).Seconds())
	return guestDesc
}
