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

package pb

import (
	"fmt"
	"strings"

	"github.com/digitalocean/go-openvswitch/ovs"
)

func (f *Flow) OvsFlow() (*ovs.Flow, error) {
	chunks := []string{}
	if f.Cookie != 0 {
		chunks = append(chunks, fmt.Sprintf("cookie=0x%x", f.Cookie))
	}
	if f.Table != 0 {
		chunks = append(chunks, fmt.Sprintf("table=%d", f.Table))
	}
	chunks = append(chunks, fmt.Sprintf("priority=%d", f.Priority))
	chunks = append(chunks, f.Matches)
	chunks = append(chunks, fmt.Sprintf("actions=%s", f.Actions))
	txt := strings.Join(chunks, ",")
	of := &ovs.Flow{}
	err := of.UnmarshalText([]byte(txt))
	if err != nil {
		return nil, err
	}
	return of, nil
}
