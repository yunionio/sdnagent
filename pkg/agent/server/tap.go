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
	"strings"
	"sync"
	"time"

	"github.com/digitalocean/go-openvswitch/ovs"

	"yunion.io/x/jsonutils"
	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"
	pkgutils "yunion.io/x/pkg/utils"

	api "yunion.io/x/onecloud/pkg/apis/compute"
	"yunion.io/x/onecloud/pkg/cloudcommon/types"
	"yunion.io/x/onecloud/pkg/mcclient/auth"
	compute_modules "yunion.io/x/onecloud/pkg/mcclient/modules/compute"
	"yunion.io/x/onecloud/pkg/util/iproute2"
	"yunion.io/x/onecloud/pkg/util/sysutils"

	"yunion.io/x/sdnagent/pkg/agent/utils"
)

var (
	vpcBridge string
)

type tapMan struct {
	agent *AgentServer
}

func newTapMan(agent *AgentServer) *tapMan {
	man := &tapMan{
		agent: agent,
	}
	return man
}

func (man *tapMan) tapBridge() string {
	return man.agent.hostConfig.TapBridgeName
}

func (man *tapMan) Start(ctx context.Context) {
	log.Infof("tap man start")

	vpcBridge = man.agent.hostConfig.OvnIntegrationBridge

	wg := ctx.Value("wg").(*sync.WaitGroup)
	defer wg.Done()

	refreshTicker := time.NewTicker(TapManRefreshRate)
	defer refreshTicker.Stop()

	for {
		select {
		case <-refreshTicker.C:
			man.refresh(ctx)
		case <-ctx.Done():
			log.Infof("tap man bye")
			return
		}
	}
}

func (man *tapMan) ensureTapBridge(ctx context.Context) error {
	{
		args := []string{
			"ovs-vsctl",
			"--", "--may-exist", "add-br", man.tapBridge(),
		}
		if err := man.exec(ctx, args); err != nil {
			return errors.Wrap(err, "eip: ensure eip bridge")
		}
	}

	if err := iproute2.NewLink(man.tapBridge()).Up().Err(); err != nil {
		return errors.Wrapf(err, "eip: set link %s up", man.tapBridge())
	}

	return nil
}

func (man *tapMan) exec(ctx context.Context, args []string) error {
	return utils.RunOvsctl(ctx, args)
}

func (man *tapMan) refresh(ctx context.Context) {
	defer log.Infoln("tap: refresh done")

	if err := man.ensureTapBridge(ctx); err != nil {
		log.Errorf("tap: ensureTapBridge: %v", err)
		return
	}
	man.run(ctx)
}

func (man *tapMan) run(ctx context.Context) {
	if len(man.agent.hostId) == 0 {
		// host id is empty
		return
	}

	err := man.syncTapConfig(ctx)
	if err != nil {
		log.Errorf("syncTapConfig fail: %s", err)
		return
	}
}

func (man *tapMan) syncTapConfig(ctx context.Context) error {
	cli := ovs.New().VSwitch

	hc := man.agent.hostConfig
	apiVer := ""
	s := auth.GetAdminSession(ctx, hc.Region, apiVer)
	cfgJson, err := compute_modules.Hosts.GetSpecific(s, man.agent.hostId, "tap-config", nil)
	if err != nil {
		return errors.Wrap(err, "Hosts.GetSpecific tap-config")
	}

	cfg := api.SHostTapConfig{}
	err = cfgJson.Unmarshal(&cfg)
	if err != nil {
		return errors.Wrap(err, "cfgJson.Unmarshal")
	}

	tapFlows := make([]*ovs.Flow, 0)
	allPorts := make([]string, 0)
	// create brtap ports
	for _, tap := range cfg.Taps {
		st := sTapService{
			tap,
		}
		if st.isGuest() {
			// make sure the tap device has been added to brtap
			// otherwise, skip the setup
			_, err := cli.PortToBridge(st.Ifname)
			if err != nil {
				log.Errorf("guest %s not belong to brtap, skip...", st.Ifname)
				continue
			}
		}
		ports, err := st.ports()
		if err != nil {
			return errors.Wrap(err, "tap.ports")
		}
		allPorts = append(allPorts, ports...)
		allArgs := st.portsArgs(cli, man.tapBridge())
		for _, args := range allArgs {
			err := man.exec(ctx, args)
			if err != nil {
				return errors.Wrap(err, "man.exec")
			}
		}
		flows, err := st.flows(man.tapBridge())
		if err != nil {
			return errors.Wrap(err, "st.flows")
		}
		tapFlows = append(tapFlows, flows...)
	}
	// remove obsolete ports from brtap
	ports, err := cli.ListPorts(man.tapBridge())
	if err != nil {
		return errors.Wrap(err, "ListPorts")
	}
	for _, p := range ports {
		if !pkgutils.IsInStringArray(p, allPorts) {
			err := cli.DeletePort(man.tapBridge(), p)
			if err != nil {
				return errors.Wrapf(err, "cli.DeletePort %s", p)
			}
		}
	}
	// sync brtap flows
	flowman := man.agent.GetFlowMan(man.tapBridge())
	flowman.updateFlows(ctx, "tapman", tapFlows)

	mirrorMap, err := fetchMirrorList(ctx)
	if err != nil {
		return errors.Wrap(err, "fetchMirrorList")
	}
	portMap, err := fetchPortNameIdMap(ctx)
	if err != nil {
		return errors.Wrap(err, "fetchPortNameIdMap")
	}
	allMirrors := make([]string, 0)
	allMirrorPorts := make([]string, 0)
	if len(cfg.Mirrors) > 0 {
		for _, m := range cfg.Mirrors {
			tm := newTapMirror(m)
			allMirrors = append(allMirrors, tm.mirrorName())
			allMirrorPorts = append(allMirrorPorts, tm.mirrorPort())
			if _, ok := mirrorMap[tm.mirrorName()]; ok {
				// mirror exists, do nothing
				continue
			}
			// prepare mirror port, create it and add it to bridge
			if br, err := cli.PortToBridge(tm.mirrorPort()); err != nil || br != tm.Bridge {
				if err == nil && br != tm.Bridge {
					// port not belong to brige, remove port
					err := cli.DeletePort(br, tm.mirrorPort())
					if err != nil {
						log.Errorf("fail to delete %s from %s:%s", tm.mirrorPort(), br, err)
					}
				}
				// add mirror port to bridge
				args := tm.mirrorPortArgs()
				err := man.exec(ctx, args)
				if err != nil {
					return errors.Wrap(err, "exec mirrorPortArgs")
				}
			}
			// setup the mirror
			args := tm.mirrorArgs()
			err = man.exec(ctx, args)
			if err != nil {
				return errors.Wrap(err, "exec mirrorArgs")
			}
		}
	}
	for m, cfg := range mirrorMap {
		if !pkgutils.IsInStringArray(m, allMirrors) {
			// need to remove mirror
			args := []string{
				"ovs-vsctl",
				"--", "--id=@m", "get", "Mirror", m,
				"--", "remove", "Bridge", cfg.Bridge, "mirrors", "@m",
			}
			err := man.exec(ctx, args)
			if err != nil {
				return errors.Wrap(err, "exec remove mirror")
			}
			// also need to remove the port, do it next
		}
	}
	for p := range portMap {
		if strings.HasPrefix(p, LocalMirrorPrefix) || strings.HasPrefix(p, RemoteMirrorPrefix) {
			if !pkgutils.IsInStringArray(p, allMirrorPorts) {
				// need to remove mirror port
				if br, err := cli.PortToBridge(p); err == nil {
					err := cli.DeletePort(br, p)
					if err != nil {
						return errors.Wrapf(err, "delete %s from %s fail %s", p, br, err)
					}
				}
			}
		}
	}

	return nil
}

type sTapService struct {
	api.STapServiceConfig
}

func (s *sTapService) isGuest() bool {
	_, err := findDevByMac(s.MacAddr)
	if err != nil {
		return true
	}
	return false
}

func findDevByMac(macAddr string) (*types.SNicDevInfo, error) {
	nics, err := sysutils.Nics()
	if err != nil {
		return nil, errors.Wrap(err, "sysutils.Nics")
	}
	for _, nic := range nics {
		if nic.Mac.String() == macAddr {
			return nic, nil
		}
	}
	return nil, errors.Wrapf(errors.ErrNotFound, "no such mac %s", macAddr)
}

func (s *sTapService) ports() ([]string, error) {
	if s.Ifname == "" {
		// physical port
		nic, err := findDevByMac(s.MacAddr)
		if err != nil {
			return nil, errors.Wrap(err, "findDevByMac")
		}
		s.Ifname = nic.Dev
	}
	ret := make([]string, 0)
	ret = append(ret, s.Ifname)
	for _, m := range s.Mirrors {
		tm := s.newTapMirror(m)
		ret = append(ret, tm.tapPort())
	}
	return ret, nil
}

func (s *sTapService) portsArgs(cli *ovs.VSwitchService, tapBridge string) [][]string {
	ret := make([][]string, 0)
	if br, err := cli.PortToBridge(s.Ifname); err != nil || br != tapBridge {
		if err == nil && br != tapBridge {
			// remove port
			err := cli.DeletePort(br, s.Ifname)
			if err != nil {
				log.Errorf("fail to delete %s from %s:%s", s.Ifname, br, err)
			}
		}
		args := []string{
			"ovs-vsctl",
			"--", "--may-exist", "add-br", tapBridge, s.Ifname,
		}
		ret = append(ret, args)
	}
	for _, m := range s.Mirrors {
		tm := s.newTapMirror(m)
		if br, err := cli.PortToBridge(tm.tapPort()); err != nil || br != tapBridge {
			if err == nil && br != tapBridge {
				// remove port
				err := cli.DeletePort(br, tm.tapPort())
				if err != nil {
					log.Errorf("fail to delete %s from %s: %s", tm.tapPort(), br, err)
				}
			}
			ret = append(ret, tm.destPortArgs(tapBridge))
		}
	}
	return ret
}

func (s *sTapService) flows(tapBridge string) ([]*ovs.Flow, error) {
	flows := make([]*ovs.Flow, 0)
	tapPort, err := utils.DumpPort(tapBridge, s.Ifname)
	if err != nil {
		return nil, errors.Wrapf(err, "utils.DumpPort %s", s.Ifname)
	}
	for _, m := range s.Mirrors {
		tm := s.newTapMirror(m)
		mf, err := tm.flow(tapBridge, tapPort)
		if err != nil {
			return nil, errors.Wrap(err, "tm.flow")
		}
		flows = append(flows, mf)
	}
	return flows, nil
}

type sTapMirror struct {
	api.SMirrorConfig
}

func newTapMirror(m api.SMirrorConfig) sTapMirror {
	tm := sTapMirror{m}
	if tm.Bridge == api.HostVpcBridge {
		tm.Bridge = vpcBridge
	}
	return tm
}

func (s *sTapService) newTapMirror(m api.SMirrorConfig) sTapMirror {
	tm := newTapMirror(m)
	tm.TapHostIp = s.TapHostIp
	return tm
}

const (
	LocalTapPrefix     = "t-loc"
	LocalMirrorPrefix  = "m-loc"
	RemoteTapPrefix    = "t-gre"
	RemoteMirrorPrefix = "m-gre"
)

func (m *sTapMirror) mirrorPort() string {
	if m.TapHostIp == m.HostIp {
		// same host, patch port
		return fmt.Sprintf("%s%04x", LocalMirrorPrefix, m.FlowId)
	} else {
		// remote host, gre port
		return fmt.Sprintf("%s%04x", RemoteMirrorPrefix, m.FlowId)
	}
}

func (m *sTapMirror) tapPort() string {
	if m.TapHostIp == m.HostIp {
		// same host, patch port
		return fmt.Sprintf("%s%04x", LocalTapPrefix, m.FlowId)
	} else {
		// remote host, gre port
		return fmt.Sprintf("%s%04x", RemoteTapPrefix, m.FlowId)
	}
}

func (m *sTapMirror) mirrorName() string {
	return fmt.Sprintf("m%04x", m.FlowId)
}

func (m *sTapMirror) destPortArgs(tapBridge string) []string {
	if m.TapHostIp == m.HostIp {
		// same host, patch port
		return []string{
			"ovs-vsctl",
			"--", "add-port", tapBridge, m.tapPort(),
			"--", "set", "interface", m.tapPort(), "type=patch", fmt.Sprintf("options:peer=%s", m.mirrorPort()),
		}
	} else {
		// remote host, gre port
		return []string{
			"ovs-vsctl",
			"--", "add-port", tapBridge, m.tapPort(),
			"--", "set", "interface", m.tapPort(), "type=gre", fmt.Sprintf("options:key=0x%x", m.FlowId), fmt.Sprintf("options:remote_ip=%s", m.HostIp),
		}
	}
}

func (m *sTapMirror) mirrorPortArgs() []string {
	if m.TapHostIp == m.HostIp {
		// same host, patch port
		return []string{
			"ovs-vsctl",
			"--", "add-port", m.Bridge, m.mirrorPort(),
			"--", "set", "interface", m.mirrorPort(), "type=patch", fmt.Sprintf("options:peer=%s", m.tapPort()),
		}
	} else {
		// remote host, gre port
		return []string{
			"ovs-vsctl",
			"--", "add-port", m.Bridge, m.mirrorPort(),
			"--", "set", "interface", m.mirrorPort(), "type=gre", fmt.Sprintf("options:key=0x%x", m.FlowId), fmt.Sprintf("options:remote_ip=%s", m.TapHostIp),
		}
	}
}

func (m *sTapMirror) flow(tapBridge string, tapPort *ovs.PortStats) (*ovs.Flow, error) {
	mPort, err := utils.DumpPort(tapBridge, m.tapPort())
	if err != nil {
		return nil, errors.Wrapf(err, "utils.DumpPort %s", m.tapPort())
	}
	return utils.F(0, 500,
		fmt.Sprintf("in_port=%d", mPort.PortID),
		fmt.Sprintf("output:%d", tapPort.PortID),
	), nil
}

func (m *sTapMirror) mirrorArgs() []string {
	args := []string{
		"ovs-vsctl",
		"--", fmt.Sprintf("--id=@%s", m.mirrorPort()), "get", "Port", m.mirrorPort(),
	}
	if len(m.Port) > 0 {
		args = append(args, "--", fmt.Sprintf("--id=@%s", m.Port), "get", "Port", m.Port)
	}
	args = append(args, "--", "--id=@m", "create", "Mirror", fmt.Sprintf("name=%s", m.mirrorName()), fmt.Sprintf("output-port=@%s", m.mirrorPort()))
	if len(m.Port) == 0 {
		// select all
		args = append(args, "select_all=true")
		if m.VlanId > 0 {
			args = append(args, fmt.Sprintf("select_vlan=%d", m.VlanId))
		}
	} else {
		if m.Direction == api.TapFlowDirectionIn || m.Direction == api.TapFlowDirectionBoth {
			args = append(args, fmt.Sprintf("select_dst_port=@%s", m.Port))
		}
		if m.Direction == api.TapFlowDirectionOut || m.Direction == api.TapFlowDirectionBoth {
			args = append(args, fmt.Sprintf("select_src_port=@%s", m.Port))
		}
	}
	args = append(args, "--", "add", "Bridge", m.Bridge, "mirrors", "@m")
	return args
}

type sMirrorConfig struct {
	Mirror string
	Bridge string
	Output string
}

// return map[mirror]bridge
func fetchMirrorList(ctx context.Context) (map[string]sMirrorConfig, error) {
	mirrorNameIdMap, err := fetchMirrorNameIdMap(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fetchMirrorNameIdMap")
	}
	mirrorIdBridgeMap, err := fetchMirrorIdBridgeMap(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fetchMirrorIdBridgeMap")
	}
	portIdName, err := fetchPortNameIdMap(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fetchPortNameIdMap")
	}
	ret := make(map[string]sMirrorConfig)
	for k, v := range mirrorNameIdMap {
		conf := sMirrorConfig{
			Mirror: k,
			Bridge: mirrorIdBridgeMap[v.Mirror],
			Output: portIdName[v.Output],
		}
		ret[k] = conf
	}
	return ret, nil
}

func fetchValue(arr jsonutils.JSONObject) string {
	idList, _ := arr.GetArray()
	if len(idList) > 1 {
		idKey, _ := idList[0].GetString()
		id, _ := idList[1].GetString()
		if idKey == "uuid" {
			return id
		}
	}
	return ""
}

func fetchPortNameIdMap(ctx context.Context) (map[string]string, error) {
	args := []string{
		"ovs-vsctl", "--format=json", "--columns=name,_uuid", "list", "Port",
	}
	output, err := utils.ExecOvsctl(ctx, args)
	if err != nil {
		return nil, errors.Wrap(err, "utils.ExecOvsctl")
	}
	return fetchPortNameIdMapInternal(output)
}

func fetchPortNameIdMapInternal(output []byte) (map[string]string, error) {
	ret := make(map[string]string)
	bridgeJson, err := jsonutils.Parse(output)
	if err != nil {
		return nil, errors.Wrap(err, "jsonutils.Parse mirror output")
	}
	dataList, err := bridgeJson.GetArray("data")
	if err != nil {
		return nil, errors.Wrap(err, "get data list")
	}
	for i := range dataList {
		data, err := dataList[i].GetArray()
		if err != nil {
			return nil, errors.Wrapf(err, "get data at %d", i)
		}
		if len(data) > 1 {
			name, _ := data[0].GetString()
			ret[name] = fetchValue(data[1])
		}
	}
	return ret, nil
}

func fetchMirrorNameIdMap(ctx context.Context) (map[string]sMirrorConfig, error) {
	args := []string{
		"ovs-vsctl", "--format=json", "--columns=name,_uuid,output_port", "list", "Mirror",
	}
	output, err := utils.ExecOvsctl(ctx, args)
	if err != nil {
		return nil, errors.Wrap(err, "utils.ExecOvsctl")
	}
	return fetchMirrorNameIdMapInternal(output)
}

func fetchMirrorNameIdMapInternal(output []byte) (map[string]sMirrorConfig, error) {
	ret := make(map[string]sMirrorConfig)
	bridgeJson, err := jsonutils.Parse(output)
	if err != nil {
		return nil, errors.Wrap(err, "jsonutils.Parse mirror output")
	}
	dataList, err := bridgeJson.GetArray("data")
	if err != nil {
		return nil, errors.Wrap(err, "get data list")
	}
	for i := range dataList {
		data, err := dataList[i].GetArray()
		if err != nil {
			return nil, errors.Wrapf(err, "get data at %d", i)
		}
		if len(data) > 1 {
			name, _ := data[0].GetString()
			ret[name] = sMirrorConfig{
				Mirror: fetchValue(data[1]),
				Output: fetchValue(data[2]),
			}
		}
	}
	return ret, nil
}

func fetchMirrorIdBridgeMap(ctx context.Context) (map[string]string, error) {
	args := []string{
		"ovs-vsctl", "--format=json", "--columns=name,mirrors", "list", "Bridge",
	}
	output, err := utils.ExecOvsctl(ctx, args)
	if err != nil {
		return nil, errors.Wrap(err, "utils.ExecOvsctl")
	}
	fmt.Println(string(output))
	return fetchMirrorIdBridgeMapInternal(output)
}

func fetchMirrorIdBridgeMapInternal(output []byte) (map[string]string, error) {
	ret := make(map[string]string)
	bridgeJson, err := jsonutils.Parse(output)
	if err != nil {
		return nil, errors.Wrap(err, "jsonutils.Parse bridge output")
	}
	dataList, err := bridgeJson.GetArray("data")
	if err != nil {
		return nil, errors.Wrap(err, "get data list")
	}
	// [["br1",["set",[]]],["br0",["set",[["uuid","5ab854d3-b050-48de-9d60-3f5791478d1c"],["uuid","d5dfa2a6-7633-4f13-89d9-ecfa2b161bda"]]]],["brtap",["set",[]]],["brmapped",["set",[]]],["breip",["set",[]]],["brvpc",["set",[]]]]
	for i := range dataList {
		// ["br0",["set",[["uuid","5ab854d3-b050-48de-9d60-3f5791478d1c"],["uuid","d5dfa2a6-7633-4f13-89d9-ecfa2b161bda"]]]]
		data, err := dataList[i].GetArray()
		if err != nil {
			return nil, errors.Wrapf(err, "get data at %d", i)
		}
		if len(data) > 1 {
			brName, _ := data[0].GetString()
			// ["set",[["uuid","5ab854d3-b050-48de-9d60-3f5791478d1c"],["uuid","d5dfa2a6-7633-4f13-89d9-ecfa2b161bda"]]]
			mirrorsList, err := data[1].GetArray()
			if err != nil {
				return nil, errors.Wrap(err, "get data mirrors")
			}
			if len(mirrorsList) > 1 {
				mirrorKey, err := mirrorsList[0].GetString()
				if err != nil {
					return nil, errors.Wrap(err, "get data mirrors name")
				}
				switch mirrorKey {
				case "uuid":
					mirrorUuid, _ := mirrorsList[1].GetString()
					ret[mirrorUuid] = brName
				case "set":
					// mirrorsList[1]: [["uuid","5ab854d3-b050-48de-9d60-3f5791478d1c"],["uuid","d5dfa2a6-7633-4f13-89d9-ecfa2b161bda"]]
					mirrorsList2, _ := mirrorsList[1].GetArray()
					for i := 0; i < len(mirrorsList2); i++ {
						// mirrorsList3: ["uuid","5ab854d3-b050-48de-9d60-3f5791478d1c"]
						mirrorsList3, _ := mirrorsList2[i].GetArray()
						mirrorUuid, _ := mirrorsList3[1].GetString()
						ret[mirrorUuid] = brName
					}
				}
			}
		}
	}
	return ret, nil
}
