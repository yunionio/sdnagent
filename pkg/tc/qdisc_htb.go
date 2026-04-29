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

	"yunion.io/x/pkg/errors"
)

const HtbDefaultClass = 65534

/*
tc qdisc add dev br0 root handle 1: htb default 65534
tc class add dev br0 parent 1: classid 1:1 htb rate 10000mbit ceil 10000mbit
tc class add dev br0 parent 1:1 classid 1:65534 htb rate 1000mbit ceil 10000mbit
tc class add dev br0 parent 1:1 classid 1:3 htb rate 100mbit ceil 100mbit
tc filter add dev br0 protocol ip parent 1: prio 1 handle 599 fw classid 1:3
*/

var _ IQdisc = &QdiscHtb{}

type QdiscHtb struct {
	*SBaseTcQdisc
	DefaultClass uint16
}

func (q *QdiscHtb) Base() *SBaseTcQdisc {
	return q.SBaseTcQdisc
}

func (q *QdiscHtb) Compare(itc IComparable) int {
	baseQdisc, ok := itc.(IQdisc)
	if !ok {
		return -1
	}
	baseCmp := q.Base().Compare(baseQdisc.Base())
	if baseCmp != 0 {
		return baseCmp
	}
	q2 := baseQdisc.(*QdiscHtb)
	if q.DefaultClass < q2.DefaultClass {
		return -1
	} else if q.DefaultClass > q2.DefaultClass {
		return 1
	}
	return 0
}

func (q *QdiscHtb) CompareBase(qi IComparable) int {
	return q.Base().CompareBase(qi.(IQdisc).Base())
}

func (q *QdiscHtb) Equals(qi IComparable) bool {
	return q.Compare(qi) == 0
}

func parseQdiscHtb(chunks []string) (*QdiscHtb, error) {
	q := &QdiscHtb{}
	for i, c := range chunks {
		switch c {
		case "default":
			if i+1 >= len(chunks) {
				return nil, errors.Wrap(errors.ErrInvalidFormat, "eol getting default class")
			}
			classId := chunks[i+1]
			base := 10
			if strings.HasPrefix(classId, "0x") {
				base = 16
				classId = classId[2:]
			}
			clsId, err := strconv.ParseUint(classId, base, 64)
			if err != nil {
				return nil, errors.Wrap(err, "strconv.ParseUint")
			}
			q.DefaultClass = uint16(clsId)
			i += 2
		default:
			i++
		}
	}
	return q, nil
}

func (q *QdiscHtb) basicLine(action string, ifname string) string {
	elms := q.SBaseTcQdisc.basicLineElements(action, ifname)
	elms = append(elms, q.Kind)
	elms = append(elms, "default", fmt.Sprintf("0x%x", q.DefaultClass))
	return strings.Join(elms, " ")
}

func (q *QdiscHtb) AddLine(ifname string) string {
	return q.basicLine("add", ifname)
}

func (q *QdiscHtb) ReplaceLine(ifname string) string {
	return q.basicLine("replace", ifname)
}
