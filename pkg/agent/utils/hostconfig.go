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
	"context"
	"fmt"
	"net"
	"reflect"
	"strings"
	"time"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"
	"yunion.io/x/pkg/util/regutils"

	apis "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/apis/identity"
	"yunion.io/x/onecloud/pkg/hostman/options"
	"yunion.io/x/onecloud/pkg/mcclient/auth"
	"yunion.io/x/onecloud/pkg/util/fileutils2"
)

type HostConfigNetwork struct {
	Bridge string
	Ifname string
	IP     net.IP
	mac    net.HardwareAddr

	IP6      net.IP
	IP6Local net.IP

	HostLocalNets []apis.NetworkDetails
}

func NewHostConfigNetwork(network string) (*HostConfigNetwork, error) {
	chunks := strings.Split(network, "/")
	if len(chunks) >= 3 {
		// the 3rd field can be an ip address or platform network name.
		// net.ParseIP will return nil when it fails
		var ip, ip6 net.IP
		if regutils.MatchIP4Addr(chunks[2]) {
			ip = net.ParseIP(chunks[2])
		} else if regutils.MatchIP6Addr(chunks[2]) {
			ip6 = net.ParseIP(chunks[2])
		}
		if len(chunks) >= 4 {
			if regutils.MatchIP4Addr(chunks[3]) {
				ip = net.ParseIP(chunks[3])
			} else if regutils.MatchIP6Addr(chunks[3]) {
				ip6 = net.ParseIP(chunks[3])
			}
		}
		return &HostConfigNetwork{
			Ifname: chunks[0],
			Bridge: chunks[1],
			IP:     ip,
			IP6:    ip6,
		}, nil
	}
	return nil, fmt.Errorf("invalid host.conf networks config: %q", network)
}

func (hcn *HostConfigNetwork) MAC() (net.HardwareAddr, error) {
	if hcn.mac == nil {
		iface, err := net.InterfaceByName(hcn.Bridge)
		if err != nil {
			return nil, errors.Wrap(err, "net.InterfaceByName")
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, errors.Wrap(err, "iface.Addrs")
		}
		var v4ip, v6ip, v6llip net.IP
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				ip4 := ipnet.IP.To4()
				ip6 := ipnet.IP.To16()
				if ip4 != nil && ip6 == nil {
					// v4 address
					if v4ip == nil || (hcn.IP != nil && ip4.String() == hcn.IP.String()) {
						v4ip = ip4
					}
				} else if ip6 != nil {
					// v6 address
					if ip6.IsLinkLocalUnicast() {
						v6llip = ip6
					} else if v6ip == nil || (hcn.IP6 != nil && ip6.String() == hcn.IP6.String()) {
						v6ip = ip6
					}
				}
			}
		}
		if v4ip == nil && v6ip == nil {
			// no either v4ip or v6ip, interface is not ready yet, return error
			return nil, errors.Wrap(errors.ErrInvalidStatus, "interface is not ready yet")
		}
		hcn.mac = iface.HardwareAddr
		if v4ip != nil {
			hcn.IP = v4ip
		}
		if v6ip != nil {
			hcn.IP6 = v6ip
		}
		if v6llip != nil {
			hcn.IP6Local = v6llip
		}
	}
	if hcn.IP != nil && hcn.mac != nil {
		return hcn.mac, nil
	}
	return nil, fmt.Errorf("cannot find proper ip/mac for %s", hcn.Bridge)
}

func (hcn *HostConfigNetwork) loadHostLocalNetconfs(hc *HostConfig) {
	log.Infof("HostConfigNetwork loadHostLocalNetconfs!!!")
	if hcn.IP == nil {
		return
	}
	fn := hc.HostLocalNetconfPath(hcn.Bridge)
	confStr, err := fileutils2.FileGetContents(fn)
	if err != nil {
		log.Warningf("fail to load host local netconfs %s: %s", fn, err)
		return
	}
	confJson, err := jsonutils.ParseString(confStr)
	if err != nil {
		log.Warningf("fail to parse host local netconfs %s: %s", fn, err)
		return
	}
	hcn.HostLocalNets = make([]apis.NetworkDetails, 0)
	err = confJson.Unmarshal(&hcn.HostLocalNets)
	if err != nil {
		log.Warningf("fail to unmarshal host local netconfs %s: %s", fn, err)
		return
	}
}

type HostConfig struct {
	options.SHostOptions

	networks []*HostConfigNetwork
}

func (hc *HostConfig) MetadataPort() int {
	return hc.Port + 1000
}

func NewHostConfig() (*HostConfig, error) {
	hostOpts := options.Parse()
	hc := &HostConfig{
		SHostOptions: hostOpts,
	}

	if hc.AllowSwitchVMs && !hc.AllowRouterVMs {
		hc.AllowRouterVMs = true
	}

	for _, network := range hc.Networks {
		hcn, err := NewHostConfigNetwork(network)
		if err != nil {
			// NOTE error ignored
			continue
		}
		hcn.loadHostLocalNetconfs(hc)
		hc.networks = append(hc.networks, hcn)
	}

	return hc, nil
}

func (hc *HostConfig) GetOverlayMTU() int {
	mtu := hc.OvnUnderlayMtu
	if mtu < 576 {
		mtu = 576
	}
	mtu -= apis.VPC_OVN_ENCAP_COST
	return mtu
}

func (hc *HostConfig) HostNetworkConfigs() []*HostConfigNetwork {
	return hc.networks
}

func (hc *HostConfig) HostNetworkConfig(bridge string) *HostConfigNetwork {
	for _, hcn := range hc.networks {
		if hcn.Bridge == bridge {
			return hcn
		}
	}
	return nil
}

func (hc *HostConfig) Equals(hc1 *HostConfig) bool {
	return reflect.DeepEqual(hc.SHostOptions, hc1.SHostOptions)
}

func (hc *HostConfig) WatchChange(ctx context.Context, cb func()) {
	tick := time.NewTicker(13 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			hc1, err := NewHostConfig()
			if err != nil {
				log.Errorf("watch host config: NewHostConfig: %v", err)
				cb()
				return
			}
			if !hc.Equals(hc1) {
				cb()
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (hc *HostConfig) Auth(ctx context.Context) error {
	a := auth.NewAuthInfo(
		hc.AuthURL,
		hc.AdminDomain,
		hc.AdminUser,
		hc.AdminPassword,
		hc.AdminProject,
		hc.AdminProjectDomain,
	)

	if t := hc.SessionEndpointType; t != "" {
		if t != identity.EndpointInterfacePublic && t != identity.EndpointInterfaceInternal {
			return fmt.Errorf("Invalid session endpoint type %q", t)
		}
		auth.SetEndpointType(t)
	}

	var (
		debugClient = false
		insecure    = true
		certfile    = hc.SslCertfile
		keyfile     = hc.SslKeyfile
	)
	if !hc.EnableSsl {
		certfile = ""
		keyfile = ""
	}
	auth.Init(a, debugClient, insecure, certfile, keyfile)
	return nil
}
