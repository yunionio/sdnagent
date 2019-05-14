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
	"os"
)

// how many ticks within one microsecond
var tickInUsec = float64(0x3e8) / float64(0x40)

func init() {
	f, err := os.Open("/proc/net/psched")
	if err != nil {
		return
	}
	var t2us, us2t, clockRes, bufferHz uint32
	n, err := fmt.Fscanf(f, "%x %x %x %x", &t2us, &us2t, &clockRes, &bufferHz)
	if n != 4 || err != nil {
		return
	}
	tickInUsec = float64(t2us) / float64(us2t)
}
