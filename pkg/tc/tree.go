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

package tc

import (
	"fmt"
	"strings"
)

type QdiscTree struct {
	qdisc    IQdisc
	children map[uint32]*QdiscTree
}

func (qt *QdiscTree) IsLeaf() bool {
	return len(qt.children) == 0
}

func (qt *QdiscTree) IsRoot() bool {
	return qt.qdisc.IsRoot()
}

func (qt *QdiscTree) String() string {
	lines := qt.BatchReplaceLines("dummy0")
	return strings.Join(lines, "\n")
}

func (qt *QdiscTree) BatchReplaceLines(ifname string) []string {
	lines := []string{}
	queue := []*QdiscTree{qt}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		lines = append(lines, cur.qdisc.ReplaceLine(ifname))
		for _, child := range cur.children {
			queue = append(queue, child)
		}
	}
	return lines
}

func (qt *QdiscTree) Equals(qt2 *QdiscTree) bool {
	if !qt.qdisc.Equals(qt2.qdisc) {
		return false
	}
	if len(qt.children) != len(qt2.children) {
		return false
	}
	for h, child := range qt.children {
		child2, ok := qt2.children[h]
		if !ok {
			return false
		}
		if !child.Equals(child2) {
			return false
		}
	}
	return true
}

func NewQdiscTree(qs []IQdisc) (*QdiscTree, error) {
	qt := &QdiscTree{
		children: map[uint32]*QdiscTree{},
	}
	for i, q := range qs {
		if q.IsRoot() {
			qt.qdisc = q
			qs = append(qs[:i], qs[i+1:]...)
			break
		}
	}
	if qt.qdisc == nil {
		err := fmt.Errorf("cannot find root qdisc")
		return nil, err
	}
	r := qt
	queue := []*QdiscTree{qt}
	for len(queue) > 0 {
		qt = queue[0]
		queue = queue[1:]
		qs0 := qs[:0]
		for _, q := range qs {
			if q.BaseQdisc().Kind == "ingress" {
				// NOTE ingress is singleton
				continue
			}
			h := qt.qdisc.BaseQdisc().Handle
			if q.BaseQdisc().Parent == h {
				qtt := &QdiscTree{
					qdisc:    q,
					children: map[uint32]*QdiscTree{},
				}
				qt.children[h] = qtt
				queue = append(queue, qtt)
			} else {
				qs0 = append(qs0, q)
			}
		}
		qs = qs0
	}
	if len(qs) > 0 {
		err := fmt.Errorf("exist orphan qdisc without parent")
		return nil, err
	}
	return r, nil
}

func NewQdiscTreeFromString(s string) (*QdiscTree, error) {
	qs := []IQdisc{}
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		q, err := NewQdiscFromString(line)
		if err != nil {
			return nil, err
		}
		qs = append(qs, q)
	}
	qt, err := NewQdiscTree(qs)
	return qt, err
}
