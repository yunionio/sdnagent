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
	"bytes"
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/digitalocean/go-openvswitch/ovs"

	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"
	pkgutils "yunion.io/x/pkg/utils"

	"yunion.io/x/onecloud/pkg/util/fileutils2"

	"yunion.io/x/sdnagent/pkg/agent/utils"
)

type flowManCmdType int

// TODO make them unique
const THEMAN string = "THEman"
const FAILSAFE string = "failSAFE"

const (
	flowManCmdAddFlow flowManCmdType = iota
	flowManCmdDelFlow
	flowManCmdSyncFlows
	flowManCmdUpdateFlows
)

type flowManCmd struct {
	Type flowManCmdType
	Who  string
	Arg  interface{}
}

type FlowMan struct {
	bridge    string
	flowSets  map[string]*utils.FlowSet
	idleTimer *time.Timer
	cmdChan   chan *flowManCmd
	waitCount int32
}

func (fm *FlowMan) doDumpFlows(excludeOvsTables []int) (*utils.FlowSet, error) {
	// check existence of ovs-db's sock file
	const ovsDbSock = "/var/run/openvswitch/db.sock"
	if !fileutils2.Exists(ovsDbSock) {
		log.Fatalf("%s not exists!", ovsDbSock)
	}
	ofCli := ovs.New().OpenFlow
	flows, err := ofCli.DumpFlows(fm.bridge)
	if err != nil {
		log.Errorf("flowman %s: dump-flows failed: %s", fm.bridge, err)
		return nil, errors.Wrap(err, "DumpFlows")
	}
	// for _, of := range flows {
	// 	utils.OVSFlowOrderMatch(of)
	// }
	{
		// filter out dynamic flow tables
		nflows := make([]*ovs.Flow, 0, len(flows))
		for i := range flows {
			if len(excludeOvsTables) > 0 && pkgutils.IsInArray(flows[i].Table, excludeOvsTables) {
				continue
			}
			nflows = append(nflows, flows[i])
		}
		flows = nflows
	}

	fs := utils.NewFlowSetFromList(flows)
	return fs, nil
}

var (
	excludeOvsTables = []int{
		10,
		12,
	}
)

func (fm *FlowMan) mergeFlows() *utils.FlowSet {
	merge := utils.NewFlowSet()
	for _, fs := range fm.flowSets {
		merge.Merge(fs)
	}
	return merge
}

func (fm *FlowMan) doCheck() {
	log.Infof("flowman %s: do check waitCount %d", fm.bridge, fm.waitCount)
	if atomic.LoadInt32(&fm.waitCount) != 0 {
		return
	}
	start := time.Now()

	defer func() {
		log.Infof("flowman %s: check done %f", fm.bridge, time.Since(start).Seconds())
	}()

	log.Infof("flowman %s: start check", fm.bridge)
	var err error
	// fs0: current flows
	fs0, err := fm.doDumpFlows(excludeOvsTables)
	if err != nil {
		log.Errorf("FlowMan doCheck doDumpFlows fail %s", err)
		return
	}
	log.Infof("flowman %s: %d flows in table", fm.bridge, fs0.Len())

	merged := fm.mergeFlows()
	log.Infof("flowman %s: %d flows in table and %d flows in memory", fm.bridge, fs0.Len(), merged.Len())
	flowsAdd, flowsDel := fs0.Diff(merged)
	fm.doCommitChange(flowsAdd, flowsDel)

	if len(flowsAdd) > 0 || len(flowsDel) > 0 {
		buf := &bytes.Buffer{}
		buf.WriteString(fmt.Sprintf("flowman %s: commit:\n", fm.bridge))
		//fm.bufWriteFlows(buf, "000-flow", fs0.Flows())
		//fm.bufWriteFlows(buf, "111-flow", flows1)
		fm.bufWriteFlows(buf, "add-flow", flowsAdd)
		fm.bufWriteFlows(buf, "del-flow", flowsDel)
		log.Infof("%s", buf.String())
	}
}

func (fm *FlowMan) bufWriteFlows(buf *bytes.Buffer, prefix string, flows []*ovs.Flow) {
	for i, f := range flows {
		txt, _ := f.MarshalText()
		buf.WriteString(fmt.Sprintf("%s:%2d: %s\n", prefix, i, txt))
	}
}

func (fm *FlowMan) doCommitChange(flowsAdd, flowsDel []*ovs.Flow) error {
	log.Infof("FlowMan %s doCommitChange flowsAdd %d flowsDel %d", fm.bridge, len(flowsAdd), len(flowsDel))
	ofCli := ovs.New(ovs.Strict(), ovs.Debug(false)).OpenFlow
	err := ofCli.AddFlowBundle(fm.bridge, func(tx *ovs.FlowTransaction) error {
		mfs := make([]*ovs.MatchFlow, len(flowsDel))
		for i, of := range flowsDel {
			mfs[i] = of.MatchFlowStrict()
			mfs[i].CookieMask = ^uint64(0)
		}
		tx.DeleteStrict(mfs...)
		tx.Add(flowsAdd...)
		return tx.Commit()
	})
	if err != nil {
		log.Errorf("flowman %s: add flow bundle failed: %s", fm.bridge, err)
		return errors.Wrapf(err, "AddFlowBundle %s", fm.bridge)
	}
	return nil
}

func (fm *FlowMan) doCmd(cmd *flowManCmd) {
	switch cmd.Type {
	case flowManCmdAddFlow:
		flow, _ := cmd.Arg.(*ovs.Flow)
		newAdd := fm.flowSets[THEMAN].Add(flow)
		if !newAdd {
			txt, _ := flow.MarshalText()
			log.Warningf("flowman %s: add-flow %s, already recorded", fm.bridge, txt)
		}
	case flowManCmdDelFlow:
		flow, _ := cmd.Arg.(*ovs.Flow)
		newDel := fm.flowSets[THEMAN].Remove(flow)
		if !newDel {
			txt, _ := flow.MarshalText()
			log.Warningf("flowman %s: del-flows %s, but not found", fm.bridge, txt)
		}
	case flowManCmdSyncFlows:
		log.Infof("flowman %s: do check command", fm.bridge)
		fm.doCheck()
		fm.scheduleIdleCheck(true)
	case flowManCmdUpdateFlows:
		// TODO check only Who using cookie
		flows, _ := cmd.Arg.([]*ovs.Flow)
		fs := utils.NewFlowSetFromList(flows)
		fm.flowSets[cmd.Who] = fs
		fm.doCheck()
		fm.scheduleIdleCheck(true)
	}
}

func (fm *FlowMan) failsafeInit() {
	fm.flowSets[FAILSAFE] = utils.NewFlowSetFromList([]*ovs.Flow{
		utils.F(0, 0, "", "normal"),
	})
}

func (fm *FlowMan) scheduleIdleCheck(drain bool) {
	if !fm.idleTimer.Stop() {
		if drain {
			<-fm.idleTimer.C
		}
	}
	fm.idleTimer.Reset(FlowManIdleCheckDuration)
}

func (fm *FlowMan) Start(ctx context.Context) {
	wg := ctx.Value("wg").(*sync.WaitGroup)
	defer wg.Done()
	if fm.idleTimer == nil {
		fm.idleTimer = time.NewTimer(FlowManIdleCheckDuration)
		defer fm.idleTimer.Stop() // just to be sure
	}
	fm.failsafeInit()
	caseCmd := reflect.SelectCase{
		Chan: reflect.ValueOf((<-chan *flowManCmd)(fm.cmdChan)),
		Dir:  reflect.SelectRecv,
	}
	caseCtx := reflect.SelectCase{
		Chan: reflect.ValueOf(ctx.Done()),
		Dir:  reflect.SelectRecv,
	}
	for {
		caseTimer := reflect.SelectCase{
			Chan: reflect.ValueOf(fm.idleTimer.C),
			Dir:  reflect.SelectRecv,
		}
		cases := []reflect.SelectCase{caseCmd, caseTimer, caseCtx}
		i, recvV, recvOk := reflect.Select(cases)
		switch cases[i] {
		case caseCmd:
			if !recvOk {
				goto out
			}
			fm.doCmd(recvV.Interface().(*flowManCmd))
		case caseTimer:
			log.Infof("flowman %s: do idle check", fm.bridge)
			fm.doCheck()
			fm.scheduleIdleCheck(false)
		case caseCtx:
			fm.doCheck()
			goto out
		}
	}
out:
	log.Infof("flowman %s: bye", fm.bridge)
}

func (fm *FlowMan) sendCmd(ctx context.Context, cmd *flowManCmd) {
	select {
	case fm.cmdChan <- cmd:
	case <-ctx.Done():
		log.Warningf("flowman %s: sendCmd ctx done: %s", fm.bridge, ctx.Err())
	}
}

func (fm *FlowMan) AddFlow(ctx context.Context, of *ovs.Flow) {
	// utils.OVSFlowOrderMatch(of)
	cmd := &flowManCmd{
		Type: flowManCmdAddFlow,
		Who:  THEMAN,
		Arg:  of,
	}
	fm.sendCmd(ctx, cmd)
}

func (fm *FlowMan) DelFlow(ctx context.Context, of *ovs.Flow) {
	// utils.OVSFlowOrderMatch(of)
	cmd := &flowManCmd{
		Type: flowManCmdDelFlow,
		Who:  THEMAN,
		Arg:  of,
	}
	fm.sendCmd(ctx, cmd)
}

func (fm *FlowMan) SyncFlows(ctx context.Context) {
	log.Debugf("flowman %s: SyncFlows", fm.bridge)
	cmd := &flowManCmd{
		Type: flowManCmdSyncFlows,
	}
	fm.sendCmd(ctx, cmd)
}

func (fm *FlowMan) updateFlows(ctx context.Context, who string, ofs []*ovs.Flow) {
	log.Debugf("flowman %s: updateFlows %s", fm.bridge, who)
	{
		v := ctx.Value("waitData")
		// The caller is responsible for coordinating access
		if wdm, ok := v.(map[string]*FlowManWaitData); ok {
			if wd, exist := wdm[fm.bridge]; !exist {
				wdm[fm.bridge] = &FlowManWaitData{
					Count:   1,
					FlowMan: fm,
				}
			} else {
				wd.Count += 1
			}
			atomic.AddInt32(&fm.waitCount, 1)
		}
	}
	// for _, of := range ofs {
	//	utils.OVSFlowOrderMatch(of)
	// }
	cmd := &flowManCmd{
		Type: flowManCmdUpdateFlows,
		Who:  who,
		Arg:  ofs,
	}
	fm.sendCmd(ctx, cmd)
}

func (fm *FlowMan) waitDecr(n int32) {
	atomic.AddInt32(&fm.waitCount, -n)
}

func NewFlowMan(bridge string) *FlowMan {
	flowSets := map[string]*utils.FlowSet{
		THEMAN:   utils.NewFlowSet(),
		FAILSAFE: utils.NewFlowSet(),
	}
	return &FlowMan{
		bridge:   bridge,
		cmdChan:  make(chan *flowManCmd),
		flowSets: flowSets,
	}
}

type FlowManWaitData struct {
	Count   int32
	FlowMan *FlowMan
}
