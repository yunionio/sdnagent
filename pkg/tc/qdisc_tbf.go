package tc

import (
	"strings"

	"yunion.io/x/pkg/errors"
)

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
	// if difference is less than 1000, ignore it
	if q.Burst < q2.Burst && q.Burst+500 < q2.Burst {
		return -1
	} else if q.Burst > q2.Burst && q.Burst-500 > q2.Burst {
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
