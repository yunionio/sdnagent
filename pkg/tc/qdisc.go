package tc

import (
	"fmt"
	"strconv"
	"strings"
)

type IQdisc interface {
	IsRoot() bool
	Initialized() error
	BaseQdisc() *Qdisc
	Equals(IQdisc) bool
	DeleteLine(ifname string) string
	ReplaceLine(ifname string) string
}

type Qdisc struct {
	Kind   string
	Handle uint32
	Parent uint32
}

func (q *Qdisc) Equals(qi IQdisc) bool {
	q2, ok := qi.(*Qdisc)
	if !ok {
		return false
	}
	if q.Kind != q2.Kind {
		return false
	}
	if q.Handle != q2.Handle {
		return false
	}
	if q.Parent != q2.Parent {
		return false
	}
	return true
}

func (q *Qdisc) BaseQdisc() *Qdisc {
	return q
}

func (q *Qdisc) IsRoot() bool {
	return q.Parent == TC_H_ROOT
}

func (q *Qdisc) Initialized() error {
	if len(q.Kind) == 0 {
		return fmt.Errorf("kind is missing")
	}
	return nil
}

func (q *Qdisc) basicLineElements(action string, ifname string) []string {
	elms := []string{"qdisc", action, "dev", ifname}
	if q.Parent == TC_H_ROOT {
		elms = append(elms, "root")
	} else {
		elms = append(elms, "parent", sprintHandle(q.Parent))
	}
	elms = append(elms, "handle", sprintHandle(q.Handle))
	return elms
}

func (q *Qdisc) replaceLineElements(ifname string) []string {
	elms := q.basicLineElements("replace", ifname)
	elms = append(elms, q.Kind)
	return elms
}

func (q *Qdisc) DeleteLine(ifname string) string {
	elms := q.basicLineElements("delete", ifname)
	line := strings.Join(elms, " ")
	return line
}
func (q *Qdisc) ReplaceLine(ifname string) string {
	elms := q.replaceLineElements(ifname)
	line := strings.Join(elms, " ")
	return line
}

type QdiscTbf struct {
	*Qdisc
	Rate    uint64
	Burst   uint64
	Cell    uint64
	Latency uint64
	Mpu     uint64
}

func (q *QdiscTbf) Equals(qi IQdisc) bool {
	q2, ok := qi.(*QdiscTbf)
	if !ok {
		return false
	}
	if !q.Qdisc.Equals(q2.Qdisc) {
		return false
	}
	if q.Rate != q2.Rate {
		return false
	}
	if q.Burst != q2.Burst {
		return false
	}
	if q.Cell != q2.Cell {
		return false
	}
	if q.Latency != q2.Latency {
		return false
	}
	if q.Mpu != q2.Mpu {
		return false
	}
	return true
}

func (q *QdiscTbf) ReplaceLine(ifname string) string {
	elms := q.Qdisc.replaceLineElements(ifname)
	elms = append(elms, "rate", PrintRate(q.Rate))
	burstCell := PrintSize(q.Burst)
	if q.Cell > 1 {
		burstCell += fmt.Sprintf("/%d", q.Cell)
	}
	elms = append(elms, "burst", burstCell)
	if q.Latency != 0 {
		elms = append(elms, "latency", PrintTime(q.Latency))
	}
	if q.Mpu != 0 {
		elms = append(elms, "mpu", PrintSize(q.Mpu))
	}
	return strings.Join(elms, " ")
}

func (q *QdiscTbf) TcNormalizeBurst() error {
	if q.Rate == 0 {
		return fmt.Errorf("rate equals zero")
	}
	burst := TcTbfBurstNormalize(q.Rate, q.Burst)
	q.Burst = burst
	return nil
}

type QdiscFqCodel struct {
	*Qdisc
}

func (q *QdiscFqCodel) Equals(qi IQdisc) bool {
	q2, ok := qi.(*QdiscFqCodel)
	if !ok {
		return false
	}
	if !q.Qdisc.Equals(q2.Qdisc) {
		return false
	}
	return true
}
func NewBaseQdisc(chunks []string) (*Qdisc, error) {
	q := &Qdisc{
		Parent: TC_H_UNSPEC,
		Handle: TC_H_UNSPEC,
	}
	for i, c := range chunks {
		switch c {
		case "qdisc":
			i++
			if i >= len(chunks) {
				return nil, fmt.Errorf("eol before getting qdisc type")
			}
			q.Kind = chunks[i]
		case "root":
			q.Parent = TC_H_ROOT
		case "handle":
			i++
			if i >= len(chunks) {
				return nil, fmt.Errorf("eol getting handle")
			}
			h, err := parseHandle(chunks[i])
			if err != nil {
				return nil, fmt.Errorf("error paring handle: %s", err)
			}
			q.Handle = h
		case "parent":
			i++
			if i >= len(chunks) {
				return nil, fmt.Errorf("eol getting parent handle")
			}
			h, err := parseHandle(chunks[i])
			if err != nil {
				return nil, fmt.Errorf("bad parent handle %s: %s", chunks[i], err)
			}
			q.Parent = h
		default:
			if q.Handle == TC_H_UNSPEC && i+1 < len(chunks) && strings.Index(chunks[i+1], ":") > 0 {
				i++
				h, err := parseHandle(chunks[i])
				if err != nil {
					return nil, fmt.Errorf("bad handle %s: %s", chunks[i], err)
				}
				q.Handle = h
			}
		}
	}
	if err := q.Initialized(); err != nil {
		return nil, err
	}
	return q, nil
}

func NewQdiscTbf(chunks []string) (*QdiscTbf, error) {
	bq, err := NewBaseQdisc(chunks)
	if err != nil {
		return nil, err
	}
	q := &QdiscTbf{Qdisc: bq}
	for i, c := range chunks {
		switch c {
		case "rate":
			i++
			if i >= len(chunks) {
				return nil, fmt.Errorf("eol getting rate")
			}
			bytesPerSec, err := ParseRate(chunks[i])
			if err != nil {
				return nil, err
			}
			q.Rate = bytesPerSec
		case "burst":
			i++
			if i >= len(chunks) {
				return nil, fmt.Errorf("eol getting burst")
			}
			v := chunks[i]
			burstCell := strings.Split(v, "/")
			bursts := burstCell[0]
			cells := "1"
			if len(burstCell) == 2 {
				cells = burstCell[1]
			} else if len(burstCell) > 2 {
				return nil, fmt.Errorf("bad burst value %s", v)
			}
			burst, err := ParseSize(bursts)
			if err != nil {
				return nil, fmt.Errorf("invalid burst size %s: %s", v, err)
			}
			cell, err := strconv.ParseUint(cells, 10, 8)
			if err != nil {
				return nil, fmt.Errorf("invalid burst cell %s: %s", v, err)
			}
			q.Burst = burst
			q.Cell = cell
		case "latency", "lat":
			i++
			if i >= len(chunks) {
				return nil, fmt.Errorf("eol getting latency")
			}
			latency, err := ParseTime(chunks[i])
			if err != nil {
				return nil, err
			}
			q.Latency = latency
		case "mpu":
			i++
			if i >= len(chunks) {
				return nil, fmt.Errorf("eol getting mpu")
			}
			bytes, err := ParseSize(chunks[i])
			if err != nil {
				return nil, err
			}
			q.Mpu = bytes
		}
	}
	err = q.TcNormalizeBurst()
	if err != nil {
		return nil, err
	}
	return q, nil
}

func NewQdiscFqCodel(chunks []string) (*QdiscFqCodel, error) {
	bq, err := NewBaseQdisc(chunks)
	if err != nil {
		return nil, err
	}
	q := &QdiscFqCodel{Qdisc: bq}
	return q, nil
}

func NewQdisc(chunks []string) (IQdisc, error) {
	bq, err := NewBaseQdisc(chunks)
	if err != nil {
		return nil, err
	}
	var q IQdisc
	switch bq.Kind {
	case "fq_codel":
		q, err = NewQdiscFqCodel(chunks)
	case "tbf":
		q, err = NewQdiscTbf(chunks)
	default:
		q = bq
	}
	return q, err
}

func NewQdiscFromString(s string) (IQdisc, error) {
	chunks := strings.Split(s, " ")
	q, err := NewQdisc(chunks)
	return q, err
}
