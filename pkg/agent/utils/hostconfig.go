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
	"strings"
	"time"

	"yunion.io/x/onecloud/pkg/hostman/options"
	"yunion.io/x/onecloud/pkg/mcclient/auth"
)

type HostConfigNetwork struct {
	Bridge string
	Ifname string
	IP     net.IP
	mac    net.HardwareAddr
}

func NewHostConfigNetwork(network string) (*HostConfigNetwork, error) {
	chunks := strings.Split(network, "/")
	if len(chunks) >= 3 {
		// the 3rd field can be an ip address or platform network name.
		// net.ParseIP will return nil when it fails
		return &HostConfigNetwork{
			Ifname: chunks[0],
			Bridge: chunks[1],
			IP:     net.ParseIP(chunks[2]),
		}, nil
	}
	return nil, fmt.Errorf("invalid host.conf networks config: %q", network)
}

func (hcn *HostConfigNetwork) IPMAC() (net.IP, net.HardwareAddr, error) {
	if hcn.mac == nil {
		iface, err := net.InterfaceByName(hcn.Bridge)
		if err != nil {
			return nil, nil, err
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, nil, err
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				ip := ipnet.IP.To4()
				if ip != nil {
					hcn.IP = ip
					break
				}
			}
		}
		hcn.mac = iface.HardwareAddr
	}
	if hcn.IP != nil && hcn.mac != nil {
		return hcn.IP, hcn.mac, nil
	}
	return nil, nil, fmt.Errorf("cannot find proper ip/mac")
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
		hc.networks = append(hc.networks, hcn)
	}

	return hc, nil
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

func (hc *HostConfig) WatchChange(ctx context.Context, cb func()) {
	tick := time.NewTicker(13 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			NewHostConfig()
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
		if t != auth.PublicEndpointType && t != auth.InternalEndpointType {
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
