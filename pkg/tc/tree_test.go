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
	"testing"
)

func TestQdiscTree(t *testing.T) {
	// output from tc qdisc show
	qstrs := []string{
		"qdisc tbf handle 1: root rate 500000Kbit burst 64000b latency 100ms mpu 64b",
		"qdisc fq_codel handle 10: parent 1:",
	}
	qs := []IQdisc{}
	for _, qstr := range qstrs {
		q, err := NewQdiscFromString(qstr)
		if err != nil {
			t.Fatalf("parse error: %s", err)
		}
		qs = append(qs, q)
	}
	qt, err := NewQdiscTree(qs)
	if err != nil {
		t.Fatalf("create qdisc tree error: %s", err)
	}
	// input for tc qdisc -batch
	batchReplaceLines := []string{
		"qdisc replace dev xxx root handle 1: tbf rate 500Mbit burst 64000b latency 100ms mpu 64b",
		"qdisc replace dev xxx parent 1: handle 10: fq_codel",
	}
	for i, line := range qt.BatchReplaceLines("xxx") {
		if line != batchReplaceLines[i] {
			t.Errorf("batch line %d:", i+1)
			t.Errorf("  want: %s", batchReplaceLines[i])
			t.Errorf("   got: %s", line)
		}
	}
}
