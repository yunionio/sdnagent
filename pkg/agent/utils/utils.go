package utils

import (
	"reflect"
	"sort"

	"github.com/digitalocean/go-openvswitch/ovs"
)

func OVSFlowOrderMatch(of *ovs.Flow) {
	// TODO more efficient sort
	sort.Slice(of.Matches, func(i, j int) bool {
		ti, _ := of.Matches[i].MarshalText()
		tj, _ := of.Matches[j].MarshalText()
		return string(ti) < string(tj)
	})
}

func OVSFlowEqual(of0, of1 *ovs.Flow) bool {
	// TODO more robust compare
	// match order
	// nil match slice and emtpy match slice
	return reflect.DeepEqual(of0, of1)
}
