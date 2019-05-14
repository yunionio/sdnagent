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
	"strconv"
	"strings"
)

// Taken from iproute2 include/linux/pkt_sched.h
const (
	TC_H_UNSPEC  uint32 = 0
	TC_H_ROOT    uint32 = 0xFFFFFFFF
	TC_H_INGRESS uint32 = 0xFFFFFFF1
	TC_H_CLSACT  uint32 = TC_H_INGRESS
)

func parseHandle(s string) (uint32, error) {
	vs := strings.SplitN(s, ":", 2)
	if len(vs) != 2 {
		return 0, fmt.Errorf("invalid handle %s", s)
	}
	IDX2NAME := [2]string{"major", "minor"}
	mm := [2]uint32{}
	for i, v := range vs {
		if i == 1 && len(v) == 0 {
			mm[i] = 0
			continue
		}
		n, err := strconv.ParseUint(v, 16, 16)
		if err != nil {
			return 0, fmt.Errorf("bad %s value %s: %s", IDX2NAME[i], v, err)
		}
		mm[i] = uint32(n)
	}
	h := (mm[0] << 16) | mm[1]
	return h, nil
}

func sprintHandle(h uint32) string {
	s := fmt.Sprintf("%x:", h>>16)
	if h&0xffff != 0 {
		s += fmt.Sprintf("%x", h&0xffff)
	}
	return s
}
