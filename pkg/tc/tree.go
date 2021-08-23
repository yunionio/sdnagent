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
	qdisc        IQdisc
	children     map[uint32]*QdiscTree
	ingressQdisc IQdisc
}

func (qt *QdiscTree) IsLeaf() bool {
	return len(qt.children) == 0
}

func (qt *QdiscTree) IsRoot() bool {
	return qt.qdisc.IsRoot()
}

func (qt *QdiscTree) Root() IQdisc {
	return qt.qdisc
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
	if qt.ingressQdisc == nil {
		if qt2.ingressQdisc != nil {
			return false
		}
	} else {
		if qt2.ingressQdisc == nil {
			return false
		}
		if !qt.ingressQdisc.Equals(qt2.ingressQdisc) {
			return false
		}
	}
	return true
}

func NewQdiscTree(qs []IQdisc) (*QdiscTree, error) {
	root := &QdiscTree{
		children: map[uint32]*QdiscTree{},
	}
	for i, q := range qs {
		if q.IsRoot() {
			root.qdisc = q
			qs = append(qs[:i], qs[i+1:]...)
			break
		}
	}
	if root.qdisc == nil {
		err := fmt.Errorf("cannot find root qdisc")
		return nil, err
	}
	var (
		trees       = []*QdiscTree{root}
		currentTree *QdiscTree
		rootqbase   = root.qdisc.BaseQdisc()
		rootqkind   = rootqbase.Kind
		rootqmaj    = rootqbase.Handle & 0xff00
	)
	for len(trees) > 0 {
		currentTree = trees[0]
		trees = trees[1:]

		var (
			qs0               = qs[:0]
			currentTreeHandle = currentTree.qdisc.BaseQdisc().Handle
		)
		for _, q := range qs {
			qbase := q.BaseQdisc()

			if qbase.Kind == "ingress" {
				// NOTE ingress is singleton
				root.ingressQdisc = q
				continue
			}
			// mq is classful
			if qbase.Parent == currentTreeHandle || (rootqkind == "mq" && qbase.Parent&0xff00 == rootqmaj) {
				qtt := &QdiscTree{
					qdisc:    q,
					children: map[uint32]*QdiscTree{},
				}
				currentTree.children[qbase.Handle] = qtt
				trees = append(trees, qtt)
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
	return root, nil
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
