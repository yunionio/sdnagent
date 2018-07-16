package utils

import (
	"sort"

	"github.com/digitalocean/go-openvswitch/ovs"
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

func (fs *FlowSet) findFlowIndex(f *ovs.Flow) int {
	i := fs.findMinIndexOfPriority(f.Priority)
	if i >= 0 {
		for j := i; j < len(fs.flows) && fs.flows[j].Priority == f.Priority; j++ {
			if OVSFlowEqual(fs.flows[j], f) {
				return j
			}
		}
		return -1
	}
	return -1
}

func (fs *FlowSet) findMinIndexOfPriority(priority int) int {
	i := sort.Search(len(fs.flows), func(i int) bool {
		return fs.flows[i].Priority >= priority
	})
	if i < len(fs.flows) {
		if fs.flows[i].Priority != priority {
			return -1
		}
		return i
	}
	return -1
}

func (fs *FlowSet) Flows() []*ovs.Flow {
	return fs.flows
}

func (fs *FlowSet) Add(f *ovs.Flow) bool {
	if !fs.Contains(f) {
		i := sort.Search(len(fs.flows), func(i int) bool {
			return fs.flows[i].Priority > f.Priority
		})
		if i < len(fs.flows) {
			fs.flows = append(fs.flows, nil)
			copy(fs.flows[i+1:], fs.flows[i:])
			fs.flows[i] = f
		} else {
			fs.flows = append(fs.flows, f)
		}
		return true
	}
	return false
}

func (fs *FlowSet) Remove(f *ovs.Flow) bool {
	i := fs.findFlowIndex(f)
	if i >= 0 {
		fs.flows = append(fs.flows[:i], fs.flows[i+1:]...)
		return true
	}
	return false
}

func (fs *FlowSet) Contains(f *ovs.Flow) bool {
	return fs.findFlowIndex(f) >= 0
}

// Diff return dels,adds that are needed to make the current set has the same
// elements as with fs1
func (fs0 *FlowSet) Diff(fs1 *FlowSet) (flowsAdd, flowsDel []*ovs.Flow) {
	flowsAdd = []*ovs.Flow{}
	flowsDel = []*ovs.Flow{}
	for _, f0 := range fs0.flows {
		if !fs1.Contains(f0) {
			flowsDel = append(flowsDel, f0)
		}
	}
	for _, f1 := range fs1.flows {
		if !fs0.Contains(f1) {
			flowsAdd = append(flowsAdd, f1)
		}
	}
	return
}

// Merge
// Sub
// Add
// AddList
