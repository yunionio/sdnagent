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
	"strings"
	"sync"
	"time"

	"yunion.io/x/log"
	"yunion.io/x/sdnagent/pkg/agent/utils"
	"yunion.io/x/sdnagent/pkg/tc"
)

const (
	tcManHostLocalWho = "hostlocal"
)

type TcManSection struct {
	// key is the ifname of tcpmanpage
	pages map[string]*utils.TcData
}

func NewTcManSection() *TcManSection {
	return &TcManSection{
		pages: make(map[string]*utils.TcData),
	}
}

func (s *TcManSection) Update(t *TcManSection) {
	s.pages = t.pages
}

type TcManCmdType int

const (
	TcManCmdAdd = iota
	TcManCmdDel
	TcManCmdSync
)

type TcManCmd struct {
	typ     TcManCmdType
	who     string
	section *TcManSection
	// if the command is executed synchronizedly
	sync bool
}

// TODO
// delete qdisc on delete who?
type TcMan struct {
	// key is id of guest or hostlocal
	// value is the config of a guest or hostlocal
	book      map[string]*TcManSection
	idleTimer *time.Ticker
	cmdChan   chan *TcManCmd
	tcCli     *tc.TcCli
}

func NewTcMan() *TcMan {
	return &TcMan{
		book:    map[string]*TcManSection{},
		tcCli:   tc.NewTcCli().Details(true).Force(true),
		cmdChan: make(chan *TcManCmd),
	}
}

func (tm *TcMan) Start(ctx context.Context) {
	wg := ctx.Value("wg").(*sync.WaitGroup)
	defer wg.Done()

	tm.idleTimer = time.NewTicker(TcManIdleCheckDuration)
	defer tm.idleTimer.Stop()
	for {
		select {
		case cmd := <-tm.cmdChan:
			tm.doCmd(ctx, cmd)
		case <-tm.idleTimer.C:
			tm.doIdleCheck(ctx)
		case <-ctx.Done():
			log.Infof("tcman bye")
			goto out
		}
	}
out:
}

func (tm *TcMan) doIdleCheck(ctx context.Context) {
	log.Infof("tcman: doing idle check")
	defer log.Infof("tcman: done idle check")
	for who, section := range tm.book {
		if who == tcManHostLocalWho {
			continue
		}
		tm.doCheckGuestSection(ctx, section)
	}
	// finally check host tc rules
	tm.doCheckHostSection(ctx, tm.book[tcManHostLocalWho])
}

func (tm *TcMan) doCheckSection(ctx context.Context, who string, section *TcManSection) {
	if who == tcManHostLocalWho {
		tm.doCheckHostSection(ctx, section)
	} else {
		tm.doCheckGuestSection(ctx, section)
	}
}

func (tm *TcMan) doCheckGuestSection(ctx context.Context, section *TcManSection) {
	for _, tcdata := range section.pages {
		tm.doCheckGuestTcData(ctx, tcdata)
	}
}

func (tm *TcMan) doCheckGuestTcData(ctx context.Context, tcdata *utils.TcData) {
	qt, err := tm.tcCli.QdiscShow(ctx, tcdata.Ifname)
	if err != nil {
		// if device does not exist, expect super man to tell us
		log.Errorf("tcman: qdisc show %s failed: %s", tcdata.Ifname, err)
		return
	}
	// if root := qt.Root(); root.BaseQdisc().Kind == "mq" {
	//	log.Infof("skip %s: it uses mq", ifname)
	//	return
	// }
	expectTree := tcdata.GuestQdiscTree()

	cmds := expectTree.Delta(qt, tcdata.Ifname)
	if len(cmds) > 0 {
		output, stderr, err := tm.tcCli.Batch(ctx, strings.Join(cmds, "\n"))
		if err != nil {
			log.Errorf("tcman: batch failed: %s cmds: %s\n%s\nstderr:\n%s", err, cmds, output, stderr)
			return
		}
		log.Infof("tcman: %s: updated qdisc\n%s\n===\n%s", tcdata.Ifname, cmds, output)
	}
}

func (tm *TcMan) doCheckHostSection(ctx context.Context, section *TcManSection) {
	for _, page := range section.pages {
		tm.doCheckHostTcData(ctx, page)
	}
}

func (tm *TcMan) doCheckHostTcData(ctx context.Context, tcdata *utils.TcData) {
	qt, err := tm.tcCli.QdiscShow(ctx, tcdata.Ifname)
	if err != nil {
		log.Errorf("tcman: qdisc show %s failed: %s", tcdata.Ifname, err)
		return
	}
	expectTree := tcdata.HostRootQdiscTree()
	rootQdisc := expectTree.RootQdisc()
	rootClass := expectTree.RootClass()
	for who, section := range tm.book {
		if who == tcManHostLocalWho {
			continue
		}
		for _, guestnicTcData := range section.pages {
			if guestnicTcData.Bridge != tcdata.Bridge {
				continue
			}
			guestNicTree := guestnicTcData.HostGuestQdiscTree(rootClass, rootQdisc)
			expectTree.Merge(guestNicTree)
		}
	}

	cmds := expectTree.Delta(qt, tcdata.Ifname)
	if len(cmds) > 0 {
		output, stderr, err := tm.tcCli.Batch(ctx, strings.Join(cmds, "\n"))
		if err != nil {
			log.Errorf("tcman: batch failed: %s cnds: %s\n%s\nstderr:\n%s", err, cmds, output, stderr)
			for _, cmd := range cmds {
				log.Debugf("tcman: %s", cmd)
			}
			return
		}
		log.Infof("tcman: %s: updated qdisc\n%s", tcdata.Ifname, output)
	}
}

func (tm *TcMan) doCmd(ctx context.Context, cmd *TcManCmd) {
	switch cmd.typ {
	case TcManCmdAdd:
		section, ok := tm.book[cmd.who]
		if !ok {
			tm.book[cmd.who] = cmd.section
			section = cmd.section
		} else {
			section.Update(cmd.section)
		}
		if cmd.sync {
			tm.doCheckSection(ctx, cmd.who, section)
		}
	case TcManCmdDel:
		delete(tm.book, cmd.who)
	case TcManCmdSync:
		tm.doIdleCheck(ctx)
	}
}

func (tm *TcMan) sendCmd(ctx context.Context, cmd *TcManCmd) {
	select {
	case tm.cmdChan <- cmd:
	case <-ctx.Done():
		log.Warningf("tcman: sendCmd ctx.Done")
		return
	}
}

func (tm *TcMan) AddIfaces(ctx context.Context, who string, data []*utils.TcData, sync bool) {
	section := &TcManSection{
		pages: map[string]*utils.TcData{},
	}
	for _, d := range data {
		section.pages[d.Ifname] = d
	}
	cmd := &TcManCmd{
		typ:     TcManCmdAdd,
		who:     who,
		section: section,
		sync:    sync,
	}
	tm.sendCmd(ctx, cmd)
}

func (tm *TcMan) ClearIfaces(ctx context.Context, who string) {
	cmd := &TcManCmd{
		typ: TcManCmdDel,
		who: who,
	}
	tm.sendCmd(ctx, cmd)
}

func (tm *TcMan) SyncAll(ctx context.Context) {
	cmd := &TcManCmd{
		typ: TcManCmdSync,
	}
	tm.sendCmd(ctx, cmd)
}
