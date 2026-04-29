package tc

import (
	"strings"
)

/*
 * modprobe ifb numifbs=0
 * ip link add dev rvnet2202-232 type ifb
 * ip link set dev rvnet2202-232 up
 * tc qdisc add dev vnet2202-232 handle ffff: ingress
 * tc filter add dev vnet2202-232 parent ffff: protocol ip u32 match u32 0 0 action mirred egress redirect dev rvnet2202-232
 * tc qdisc add dev rvnet2202-232 root tbf rate 30mbit burst 32kbit latency 100ms
 */

var _ IQdisc = &QdiscIngress{}

type QdiscIngress struct {
	*SBaseTcQdisc
}

func (q *QdiscIngress) Base() *SBaseTcQdisc {
	return q.SBaseTcQdisc
}

func (q *QdiscIngress) Compare(itc IComparable) int {
	baseQdisc, ok := itc.(IQdisc)
	if !ok {
		return -1
	}
	baseCmp := q.Base().Compare(baseQdisc.Base())
	if baseCmp != 0 {
		return baseCmp
	}
	return 0
}

func (q *QdiscIngress) CompareBase(qi IComparable) int {
	return q.Base().CompareBase(qi.(IQdisc).Base())
}

func (q *QdiscIngress) Equals(qi IComparable) bool {
	return q.Compare(qi) == 0
}

func parseQdiscIngress(chunks []string) (*QdiscIngress, error) {
	q := &QdiscIngress{}
	return q, nil
}

func (q *QdiscIngress) basicLine(action string, ifname string) string {
	elms := q.SBaseTcQdisc.basicLineElements(action, ifname)
	elms = append(elms, q.Kind)
	return strings.Join(elms, " ")
}

func (q *QdiscIngress) AddLine(ifname string) string {
	return q.basicLine("add", ifname)
}

func (q *QdiscIngress) ReplaceLine(ifname string) string {
	return q.basicLine("replace", ifname)
}
