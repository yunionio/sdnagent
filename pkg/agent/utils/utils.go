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
	"sort"
	"strings"

	"github.com/digitalocean/go-openvswitch/ovs"

	"yunion.io/x/log"
	"yunion.io/x/pkg/utils"
)

/*func OVSFlowOrderMatch(of *ovs.Flow) {
	// TODO more efficient sort
	sort.Slice(of.Matches, func(i, j int) bool {
		ti, _ := of.Matches[i].MarshalText()
		tj, _ := of.Matches[j].MarshalText()
		return string(ti) < string(tj)
	})
}*/

func OVSFlowEqual(of0, of1 *ovs.Flow) bool {
	// TODO more robust compare
	// match order
	// nil match slice and emtpy match slice
	// return reflect.DeepEqual(of0, of1)
	cmp := CompareOVSFlow(of0, of1)
	return cmp == 0
}

func ovsMatchString(match ovs.Match) string {
	buf, err := match.MarshalText()
	if err != nil {
		log.Errorf("ovsMatchString MarshalText fail %s", err)
		return match.GoString()
	}
	str := string(buf)
	if utils.IsInArray(str, []string{
		"nw_src=0.0.0.0/0",
		"nw_dst=0.0.0.0/0",
		"ipv6_src=::/0",
		"ipv6_dst=::/0",
	}) {
		return ""
	}
	return str
}

func ovsMatchStrings(matches []ovs.Match) []string {
	strs := make([]string, 0, len(matches))
	for i := range matches {
		result := ovsMatchString(matches[i])
		if len(result) > 0 {
			strs = append(strs, result)
		}
	}
	sort.Strings(strs)
	return strs
}

func cmpOvsMatches(m1, m2 []ovs.Match) int {
	s1 := ovsMatchStrings(m1)
	s2 := ovsMatchStrings(m2)

	if len(s1) < len(s2) {
		return -1
	} else if len(s1) > len(s2) {
		return 1
	}
	for i := range s1 {
		cmp := strings.Compare(s1[i], s2[i])
		if cmp != 0 {
			return cmp
		}
	}
	return 0
}

func ovsActionString(action ovs.Action) string {
	str, err := action.MarshalText()
	if err != nil {
		log.Errorf("ovsActionString MarshalText %s", err)
		return action.GoString()
	}
	return string(str)
}

func ovsActionStrings(actions []ovs.Action) []string {
	strs := make([]string, 0, len(actions))
	for i := range actions {
		result := ovsActionString(actions[i])
		if len(result) > 0 {
			strs = append(strs, result)
		}
	}
	return strs
}

func cmpOvsActions(m1, m2 []ovs.Action) int {
	s1 := ovsActionStrings(m1)
	s2 := ovsActionStrings(m2)

	if len(s1) < len(s2) {
		return -1
	} else if len(s1) > len(s2) {
		return 1
	}
	for i := range s1 {
		cmp := strings.Compare(s1[i], s2[i])
		if cmp != 0 {
			return cmp
		}
	}
	return 0
}

/*
 * Priority    int
 * Protocol    Protocol
 * InPort      int
 * Matches     []Match
 * Table       int
 * IdleTimeout int
 * Cookie      uint64
 * Actions     []Action
 */
func CompareOVSFlow(flow1, flow2 *ovs.Flow) int {
	// compare table, the less, the less
	if flow1.Table < flow2.Table {
		return -1
	} else if flow1.Table > flow2.Table {
		return 1
	}
	// compare Priority, the higher, the less
	if flow1.Priority > flow2.Priority {
		return -1
	} else if flow1.Priority < flow2.Priority {
		return 1
	}
	// compare in_port, the less, the less
	if flow1.InPort < flow2.InPort {
		return -1
	} else if flow1.InPort > flow2.InPort {
		return 1
	}
	// compare protocol
	protoCmp := strings.Compare(string(flow1.Protocol), string(flow2.Protocol))
	if protoCmp != 0 {
		return protoCmp
	}
	// compare matches
	matchCmp := cmpOvsMatches(flow1.Matches, flow2.Matches)
	if matchCmp != 0 {
		return matchCmp
	}
	// compare actions
	actionCmp := cmpOvsActions(flow1.Actions, flow2.Actions)
	if actionCmp != 0 {
		return actionCmp
	}
	// compare idel_timeout
	if flow1.IdleTimeout < flow2.IdleTimeout {
		return -1
	} else if flow1.IdleTimeout > flow2.IdleTimeout {
		return 1
	}
	// compare cookie
	if flow1.Cookie < flow2.Cookie {
		return -1
	} else if flow1.Cookie > flow2.Cookie {
		return 1
	}
	return 0
}
