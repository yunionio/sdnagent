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
	"strings"

	"github.com/digitalocean/go-openvswitch/ovs"
	"yunion.io/x/pkg/errors"
)

type FlowSet struct {
	flows []*ovs.Flow
}

func NewFlowSet() *FlowSet {
	return &FlowSet{flows: []*ovs.Flow{}}
}

func NewFlowSetFromList(flows []*ovs.Flow) *FlowSet {
	fs := NewFlowSet()
	for _, of := range flows {
		fs.Add(of)
	}
	return fs
}

func (fs *FlowSet) findFlowIndex(f *ovs.Flow) (int, bool) {
	i := 0
	j := len(fs.flows) - 1
	for i <= j {
		m := (i + j) / 2
		result := CompareOVSFlow(fs.flows[m], f)
		if result < 0 {
			i = m + 1
		} else if result > 0 {
			j = m - 1
		} else {
			return m, true
		}
	}
	return j + 1, false
}

func (fs *FlowSet) Flows() []*ovs.Flow {
	return fs.flows
}

func (fs *FlowSet) Add(f *ovs.Flow) bool {
	i, find := fs.findFlowIndex(f)
	if !find {
		fs.flows = append(fs.flows, f)
		copy(fs.flows[i+1:], fs.flows[i:])
		fs.flows[i] = f
		return true
	}
	return false
}

func (fs *FlowSet) Remove(f *ovs.Flow) bool {
	i, find := fs.findFlowIndex(f)
	if find {
		fs.flows = append(fs.flows[:i], fs.flows[i+1:]...)
		return true
	}
	return false
}

func (fs *FlowSet) Contains(f *ovs.Flow) bool {
	_, find := fs.findFlowIndex(f)
	return find
}

func (fs *FlowSet) DumpFlows() (string, error) {
	buf := strings.Builder{}
	for _, f := range fs.flows {
		strf, err := f.MarshalText()
		if err != nil {
			return "", errors.Wrap(err, "MarshalText")
		}
		buf.WriteString(string(strf))
		buf.WriteByte('\n')
	}
	return buf.String(), nil
}

/*func (fs *FlowSet) Merge(fs1 *FlowSet) {
	for _, f := range fs1.flows {
		fs.Add(f)
	}
}*/

// Diff return dels,adds that are needed to make the current set has the same
// elements as with fs1
/*func (fs0 *FlowSet) Diff(fs1 *FlowSet) (flowsAdd, flowsDel []*ovs.Flow) {
	flowsAdd = []*ovs.Flow{}
	flowsDel = []*ovs.Flow{}

	i := 0
	j := 0
	for i < len(fs0.flows) && j < len(fs1.flows) {
		cmp := CompareOVSFlow(fs0.flows[i], fs1.flows[j])
		if cmp < 0 {
			flowsDel = append(flowsDel, fs0.flows[i])
			i += 1
		} else if cmp > 0 {
			flowsAdd = append(flowsAdd, fs1.flows[j])
			j += 1
		} else {
			i += 1
			j += 1
		}
	}
	if i < len(fs0.flows) {
		flowsDel = append(flowsDel, fs0.flows[i:]...)
	}
	if j < len(fs1.flows) {
		flowsAdd = append(flowsAdd, fs1.flows[j:]...)
	}
	return
}*/
