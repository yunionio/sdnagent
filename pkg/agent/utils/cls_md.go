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

	"yunion.io/x/log"

	"yunion.io/x/onecloud/pkg/cloudcommon/app"
	common_options "yunion.io/x/onecloud/pkg/cloudcommon/options"
	"yunion.io/x/onecloud/pkg/hostman/guestman/desc"
	"yunion.io/x/onecloud/pkg/hostman/metadata"
	"yunion.io/x/onecloud/pkg/util/netutils2"
)

const (
	fakeMdSrcIpPrefix   = "100.%d.%d.0/20"
	fakeMdSrcIpPattern  = "100.%d.%d.%d"
	fakeMdSrcMacPattern = "ee:ee:ee:%02x:%02x:00"
)

func (h *HostLocal) fakeMdSrcIpMac(port int) (string, string, string) {
	return fakeMdSrcIpMac(h.IP.String(), h.MAC.String(), port)
}

func fakeMdSrcIpMac(ip, mac string, port int) (string, string, string) {
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

func (h *HostLocal) StartMetadataServer(watcher IServerWatcher) {
	sport := h.HostConfig.MetadataPort() + 1
	for port := sport; port < sport+20; port++ {
		if !netutils2.IsTcpPortUsed(h.IP.String(), port) {
			h.metadataPort = port
		}
	}
	svc := &metadata.Service{
		Address: h.IP.String(),
		Port:    h.metadataPort,

		DescGetter: &sClassicMetadataDescGetter{
			hostLocal: h,
			watcher:   watcher,
		},
	}
	dbAccess := false
	h.metadataApp = app.InitApp(&common_options.BaseOptions{
		ApplicationID:      "metadata-server-class-network",
		RequestWorkerCount: 1,
	}, dbAccess)
	log.Infof("Start metadata server at %s on port %s:%d", h.Bridge, h.IP.String(), h.metadataPort)
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
	guestDesc := g.watcher.FindGuestDescByHostLocalIp(g.hostLocal, ip)
	return guestDesc
}
