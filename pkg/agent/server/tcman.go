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

	"github.com/vishvananda/netlink"

	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"
	"yunion.io/x/sdnagent/pkg/agent/utils"
	"yunion.io/x/sdnagent/pkg/tc"
)

type TcManPage struct {
	ifname string
	data   *utils.TcData
	qtWant *tc.QdiscTree
	qtGot  *tc.QdiscTree
}

func (p *TcManPage) batchReplaceInput() string {
	// lines := []string{"qdisc del dev " + p.ifname + " root"}
	lines := []string{}
	lines = append(lines, p.qtWant.BatchReplaceLines(p.ifname)...)
	lines = append(lines, "qdisc show dev "+p.ifname)
	input := strings.Join(lines, "\n")
	// NOTE final newline is needed to workaround buffer overflow issues in
	// earlier versions of tc
	//
	// - utils: fix makeargs stack overflowm,
	//   https://github.com/shemminger/iproute2/commit/bd9cea5d8c9dc6266f9529e1be6bc7dab4519d9c
	input += "\n"
	return input
}

func (p *TcManPage) deleteDevQdiscs(ifname string) error {
	link, err := netlink.LinkByName(ifname)
	if err != nil {
		return errors.Wrapf(err, "get iface by name: %s", ifname)
	}
	qdiscs, err := netlink.QdiscList(link)
	if err != nil {
		return errors.Wrapf(err, "get iface qdiscs: %s", ifname)
	}

	errs := []error{}
	for _, qdisc := range qdiscs {
		if err := netlink.QdiscDel(qdisc); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.NewAggregate(errs)
}

type TcManSection struct {
	pages map[string]*TcManPage
}

func NewTcManSection() *TcManSection {
	return &TcManSection{
		pages: map[string]*TcManPage{},
	}
}

func (s *TcManSection) Update(t *TcManSection) {
	pages0 := s.pages
	pages1 := t.pages
	for ifname, page0 := range pages0 {
		page1, ok := pages1[ifname]
		if ok {
			if !page0.qtWant.Equals(page1.qtWant) {
				pages0[ifname] = page1
			}
		} else {
			delete(pages0, ifname)
		}
	}
	for ifname, page1 := range pages1 {
		_, ok := pages0[ifname]
		if !ok {
			pages0[ifname] = page1
		}
	}
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
	for _, section := range tm.book {
		tm.doCheckSection(ctx, section)
	}
}

func (tm *TcMan) doCheckSection(ctx context.Context, section *TcManSection) {
	for _, page := range section.pages {
		tm.doCheckPage(ctx, page)
	}
}

func (tm *TcMan) doCheckPage(ctx context.Context, page *TcManPage) {
	ifname := page.ifname
	qt, err := tm.tcCli.QdiscShow(ctx, ifname)
	if err != nil {
		// if device does not exist, expect super man to tell us
		log.Errorf("tcman: qdisc show %s failed: %s", ifname, err)
		return
	}
	if root := qt.Root(); root.BaseQdisc().Kind == "mq" {
		log.Infof("skip %s: it uses mq", ifname)
		return
	}
	if qt.Equals(page.qtWant) {
		if page.qtGot == nil {
			page.qtGot = qt
		}
		return
	}
	if page.qtGot != nil && qt.Equals(page.qtGot) {
		return
	}

	// TODO batch them all
	if err := page.deleteDevQdiscs(page.ifname); err != nil {
		log.Warningf("delete ifname %s all qdiscs before batch replace: %v", page.ifname, err)
	}
	input := page.batchReplaceInput()
	output, stderr, err := tm.tcCli.Batch(ctx, input)
	if err != nil {
		log.Errorf("tcman: batch failed: %s\n%s\nstderr:\n%s", err, input, stderr)
		return
	}
	log.Infof("tcman: %s: updated qdisc\n%s\n===\n%s", ifname, input, output)
	qt, err = tc.NewQdiscTreeFromString(output)
	if err != nil {
		log.Errorf("tcman: parse qdisc tree failed: %s\n%s", err, output)
		return
	}
	page.qtGot = qt
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
			tm.doCheckSection(ctx, section)
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
		pages: map[string]*TcManPage{},
	}
	for _, d := range data {
		qt, err := d.QdiscTree()
		if err != nil {
			log.Errorf("tcman: making qdisc tree from tcdata(%q) failed: %s", d, err)
			return
		}
		ifname := d.Ifname
		page := &TcManPage{
			ifname: ifname,
			data:   d,
			qtWant: qt,
		}
		section.pages[ifname] = page
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
