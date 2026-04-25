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
	"strings"

	"yunion.io/x/pkg/errors"
)

type QdiscTree struct {
	qdisc   []IQdisc
	classes []IClass
	filters []IFilter
}

func (qt *QdiscTree) RootQdisc() IQdisc {
	for i := range qt.qdisc {
		if qt.qdisc[i].IsRoot() {
			return qt.qdisc[i]
		}
	}
	return nil
}

func (qt *QdiscTree) RootClass() IClass {
	for i := range qt.classes {
		if qt.classes[i].IsRoot() {
			return qt.classes[i]
		}
	}
	return nil
}

func (qt *QdiscTree) Merge(qt2 *QdiscTree) {
	qt.qdisc = append(qt.qdisc, qt2.qdisc...)
	qt.classes = append(qt.classes, qt2.classes...)
	qt.filters = append(qt.filters, qt2.filters...)
	Sort(qt.qdisc)
	Sort(qt.classes)
	Sort(qt.filters)
}

func (qt *QdiscTree) Delta(qt2 *QdiscTree, ifname string) []string {
	lines := []string{}
	addedQdisc, updatedQdisc1, updatedQdisc2, removedQdisc := Split(qt.qdisc, qt2.qdisc, true)
	addedClass, updatedClass1, updatedClass2, removedClass := Split(qt.classes, qt2.classes, true)
	addedFilter, _, _, removedFilter := Split(qt.filters, qt2.filters, false)
	for i := len(removedFilter) - 1; i >= 0; i-- {
		lines = append(lines, removedFilter[i].DeleteLine(ifname))
	}
	for i := len(removedClass) - 1; i >= 0; i-- {
		lines = append(lines, removedClass[i].DeleteLine(ifname))
	}
	for i := len(removedQdisc) - 1; i >= 0; i-- {
		lines = append(lines, removedQdisc[i].DeleteLine(ifname))
	}
	for i := range updatedQdisc1 {
		if updatedQdisc2[i].Equals(updatedQdisc1[i]) {
			continue
		}
		lines = append(lines, updatedQdisc1[i].ReplaceLine(ifname))
	}
	for i := range updatedClass1 {
		if updatedClass2[i].Equals(updatedClass1[i]) {
			continue
		}
		lines = append(lines, updatedClass1[i].ReplaceLine(ifname))
	}
	for i := range addedQdisc {
		lines = append(lines, addedQdisc[i].AddLine(ifname))
	}
	for i := range addedClass {
		lines = append(lines, addedClass[i].AddLine(ifname))
	}
	for i := range addedFilter {
		lines = append(lines, addedFilter[i].AddLine(ifname))
	}
	return lines
}

func NewQdiscTree(qs []IQdisc, cls []IClass, filters []IFilter) *QdiscTree {
	Sort(qs)
	Sort(cls)
	Sort(filters)
	return &QdiscTree{
		qdisc:   qs,
		classes: cls,
		filters: filters,
	}
}

func NewQdiscTreeFromString(qdisc string, classStr string, filterStr string) (*QdiscTree, error) {
	qs, err := parseQdiscLines(strings.Split(qdisc, "\n"))
	if err != nil {
		return nil, errors.Wrap(err, "parse qdisc lines")
	}
	cls, err := parseClassLines(strings.Split(classStr, "\n"))
	if err != nil {
		return nil, errors.Wrap(err, "parse class lines")
	}
	filters, err := parseFilterLines(strings.Split(filterStr, "\n"), qs)
	if err != nil {
		return nil, errors.Wrap(err, "parse filter lines")
	}
	return NewQdiscTree(qs, cls, filters), nil
}
