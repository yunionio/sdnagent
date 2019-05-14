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
