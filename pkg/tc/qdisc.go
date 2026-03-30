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
	"strings"

	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"
)

/*
 * // add
 * tc qdisc add dev br0 root handle 1: htb default 2
 * tc qdisc add dev eth0 root tbf rate 512kbit burst 10kb latency 70ms
 * // show
 * qdisc htb 1: root refcnt 2 r2q 10 default 0x2 direct_packets_stat 6 direct_qlen 1000
 * qdisc clsact ffff: parent ffff:fff1
 * qdisc tbf 1: root refcnt 2 rate 100Mbit burst 12500b lat 100ms
 * qdisc fq_codel 10: parent 1: limit 10240p flows 1024 quantum 1514 target 5ms interval 100ms memory_limit 32Mb ecn drop_batch 64
 */
type IQdisc interface {
	ITcObj
	ITcObjAlter

	Base() *SBaseTcQdisc

	IsRoot() bool
}

type SBaseTcQdisc struct {
	Kind   string
	Handle string
	Parent string
}

func (q *SBaseTcQdisc) Id() string {
	return q.Handle
}

func (q *SBaseTcQdisc) Compare(qi IComparable) int {
	q2, ok := qi.(*SBaseTcQdisc)
	if !ok {
		return -1
	}
	if q.Handle != q2.Handle {
		return compareClassId(q.Handle, q2.Handle)
	}
	if q.Kind != q2.Kind {
		return strings.Compare(q.Kind, q2.Kind)
	}
	if q.Parent != q2.Parent {
		return strings.Compare(q.Parent, q2.Parent)
	}
	return 0
}

func (q *SBaseTcQdisc) CompareBase(qi IComparable) int {
	return q.Compare(qi)
}

func (q *SBaseTcQdisc) Equals(q2 IComparable) bool {
	return q.Compare(q2) == 0
}

func (q *SBaseTcQdisc) BaseQdisc() *SBaseTcQdisc {
	return q
}

func (q *SBaseTcQdisc) IsRoot() bool {
	return len(q.Parent) == 0
}

func (q *SBaseTcQdisc) Base() *SBaseTcQdisc {
	return q
}

func (q *SBaseTcQdisc) Initialized() error {
	if len(q.Kind) == 0 {
		return fmt.Errorf("kind is missing")
	}
	return nil
}

func (q *SBaseTcQdisc) basicLineElements(action string, ifname string) []string {
	elms := []string{"qdisc", action, "dev", ifname}
	if len(q.Parent) == 0 {
		elms = append(elms, "root")
	} else {
		elms = append(elms, "parent", q.Parent)
	}
	elms = append(elms, "handle", q.Handle)
	return elms
}

func (q *SBaseTcQdisc) DeleteLine(ifname string) string {
	elms := q.basicLineElements("delete", ifname)
	line := strings.Join(elms, " ")
	return line
}

type QdiscTbf struct {
	*SBaseTcQdisc
	Rate    uint64
	Burst   uint64
	Latency uint64
}

func (q *QdiscTbf) Base() *SBaseTcQdisc {
	return q.SBaseTcQdisc
}

func (q *QdiscTbf) Compare(itc IComparable) int {
	baseQdisc, ok := itc.(IQdisc)
	if !ok {
		return -1
	}
	baseCmp := q.Base().Compare(baseQdisc.Base())
	if baseCmp != 0 {
		return baseCmp
	}
	q2 := baseQdisc.(*QdiscTbf)
	if q.Rate < q2.Rate {
		return -1
	} else if q.Rate > q2.Rate {
		return 1
	}
	if q.Burst < q2.Burst {
		return -1
	} else if q.Burst > q2.Burst {
		return 1
	}
	if q.Latency < q2.Latency {
		return -1
	} else if q.Latency > q2.Latency {
		return 1
	}
	return 0
}

func (q *QdiscTbf) CompareBase(qi IComparable) int {
	return q.Base().CompareBase(qi.(IQdisc).Base())
}

func (q *QdiscTbf) Equals(qi IComparable) bool {
	return q.Compare(qi) == 0
}

func (q *QdiscTbf) basicLine(action string, ifname string) string {
	elms := q.SBaseTcQdisc.basicLineElements(action, ifname)
	elms = append(elms, q.Kind)
	elms = append(elms, "rate", PrintRate(q.Rate))
	burstCell := PrintSize(q.Burst)
	elms = append(elms, "burst", burstCell)
	if q.Latency != 0 {
		elms = append(elms, "latency", PrintTime(q.Latency))
	}
	return strings.Join(elms, " ")
}

func (q *QdiscTbf) AddLine(ifname string) string {
	return q.basicLine("add", ifname)
}

func (q *QdiscTbf) ReplaceLine(ifname string) string {
	return q.basicLine("replace", ifname)
}

func parseBaseQdisc(chunks []string) (*SBaseTcQdisc, error) {
	q := &SBaseTcQdisc{}
	for i := 0; i < len(chunks); {
		c := chunks[i]
		switch c {
		case "qdisc":
			if i+2 >= len(chunks) {
				return nil, fmt.Errorf("eol before getting qdisc type")
			}
			q.Kind = chunks[i+1]
			q.Handle = chunks[i+2]
			i += 3
		case "root":
			q.Parent = ""
			i++
		case "parent":
			if i+1 >= len(chunks) {
				return nil, fmt.Errorf("eol getting parent handle")
			}
			q.Parent = chunks[i+1]
			i += 2
		default:
			i++
		}
	}
	if err := q.Initialized(); err != nil {
		return nil, err
	}
	return q, nil
}

func parseQdiscTbf(chunks []string) (*QdiscTbf, error) {
	q := &QdiscTbf{}
	for i, c := range chunks {
		switch c {
		case "rate":
			if i+1 >= len(chunks) {
				return nil, errors.Wrap(errors.ErrInvalidFormat, "eol getting rate")
			}
			bytesPerSec, err := ParseRate(chunks[i+1])
			if err != nil {
				return nil, errors.Wrap(err, "ParseRate")
			}
			q.Rate = bytesPerSec
			i += 2
		case "burst":
			if i+1 >= len(chunks) {
				return nil, errors.Wrap(errors.ErrInvalidFormat, "eol getting burst")
			}
			bursts := chunks[i+1]
			if strings.Contains(bursts, "/") {
				burstCell := strings.Split(bursts, "/")
				bursts = burstCell[0]
			}
			burst, err := ParseSize(bursts)
			if err != nil {
				return nil, errors.Wrap(err, "ParseSize")
			}
			q.Burst = burst
			i += 2
		case "latency", "lat":
			if i+1 >= len(chunks) {
				return nil, errors.Wrap(errors.ErrInvalidFormat, "eol getting latency")
			}
			latency, err := ParseTime(chunks[i+1])
			if err != nil {
				return nil, errors.Wrap(err, "ParseTime")
			}
			q.Latency = latency
			i += 2
		default:
			i++
		}
	}
	// err := q.TcNormalizeBurst()
	// if err != nil {
	// 	return nil, errors.Wrap(err, "TcNormalizeBurst")
	// }
	return q, nil
}

func parseQdisc(chunks []string) (IQdisc, error) {
	bq, err := parseBaseQdisc(chunks)
	if err != nil {
		return nil, err
	}
	switch bq.Kind {
	case "htb":
		q, err := parseQdiscHtb(chunks)
		if err != nil {
			return nil, errors.Wrap(err, "parseQdiscHtb")
		}
		q.SBaseTcQdisc = bq
		return q, nil
	case "tbf":
		q, err := parseQdiscTbf(chunks)
		if err != nil {
			return nil, errors.Wrap(err, "parseQdiscTbf")
		}
		q.SBaseTcQdisc = bq
		return q, nil
	}
	return nil, errors.Wrap(errors.ErrInvalidFormat, "unknown qdisc type")
}

func parseQdiscLines(lines []string) ([]IQdisc, error) {
	qs := []IQdisc{}
	for _, line := range lines {
		chunks := strings.Split(strings.TrimSpace(line), " ")
		q, err := parseQdisc(chunks)
		if err != nil {
			log.Errorf("parseQdisc %s failed: %s", line, err)
		} else {
			qs = append(qs, q)
		}
	}
	return qs, nil
}
