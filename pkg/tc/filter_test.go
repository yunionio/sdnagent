package tc

import (
	"reflect"
	"testing"

	"yunion.io/x/jsonutils"
)

func TestParseFilterLines(t *testing.T) {
	parentHtbQdisc := &QdiscHtb{
		SBaseTcQdisc: &SBaseTcQdisc{
			Kind:   "htb",
			Handle: "1:",
			Parent: "",
		},
		DefaultClass: 0,
	}
	parentIngressQdisc := &QdiscIngress{
		SBaseTcQdisc: &SBaseTcQdisc{
			Kind:   "ingress",
			Handle: "ffff:",
			Parent: "",
		},
	}
	cases := []struct {
		parent      IQdisc
		ifname      string
		in          []string
		want        []IFilter
		delLine     []string
		replaceLine []string
	}{
		{
			parent: parentHtbQdisc,
			ifname: "eth0",
			in: []string{
				"filter parent 1: protocol ip pref 1 fw chain 0",
				"filter parent 1: protocol ip pref 1 fw chain 0 handle 0x257 classid 1:3",
			},
			want: []IFilter{
				&SFwFilter{
					SBaseTcFilter: &SBaseTcFilter{
						Kind:     "fw",
						Prio:     1,
						Protocol: "ip",
						Parent:   parentHtbQdisc,
					},
					ClassId: "1:3",
					Handle:  0x257,
				},
			},
			delLine: []string{
				"filter delete dev eth0 parent 1: protocol ip prio 1 handle 0x257 fw classid 1:3",
			},
			replaceLine: []string{
				"filter add dev eth0 parent 1: protocol ip prio 1 handle 0x257 fw classid 1:3",
			},
		},
		{
			parent: parentIngressQdisc,
			ifname: "eth0",
			in: []string{
				"filter parent ffff: protocol ip pref 49152 u32 chain 0",
				"filter parent ffff: protocol ip pref 49152 u32 chain 0 fh 800: ht divisor 1",
				"filter parent ffff: protocol ip pref 49152 u32 chain 0 fh 800::800 order 2048 key ht 800 bkt 0 terminal flowid ??? not_in_hw",
				"   match 00000000/00000000 at 0",
				"   action order 1: mirred (Egress Redirect to device reth0) stolen",
				"   index 1 ref 1 bind 1",
			},
			want: []IFilter{
				&SU32Filter{
					SBaseTcFilter: &SBaseTcFilter{
						Kind:     "u32",
						Prio:     49152,
						Protocol: "ip",
						Parent:   parentIngressQdisc,
					},
					RedirectDev: "reth0",
				},
			},
			delLine: []string{
				"filter delete dev eth0 parent ffff: protocol ip prio 49152 u32 match u32 0 0 action mirred egress redirect dev reth0",
			},
			replaceLine: []string{
				"filter add dev eth0 parent ffff: protocol ip prio 49152 u32 match u32 0 0 action mirred egress redirect dev reth0",
			},
		},
	}
	for _, c := range cases {
		filters, err := parseFilterLines(c.in, []IQdisc{c.parent})
		if err != nil {
			t.Errorf("%s", err)
			continue
		}
		if len(filters) != len(c.want) {
			t.Errorf("want %d filters, got %d", len(c.want), len(filters))
			continue
		}
		for i := range filters {
			if !filters[i].Equals(c.want[i]) {
				t.Errorf("filter %d: want %v, got %v", i, jsonutils.Marshal(c.want[i]), jsonutils.Marshal(filters[i]))
				continue
			}
		}
		delLines := []string{}
		for _, filter := range filters {
			delLines = append(delLines, filter.DeleteLine(c.ifname))
		}
		if !reflect.DeepEqual(delLines, c.delLine) {
			t.Errorf("want %v, got %v", c.delLine, delLines)
		}
		replaceLines := []string{}
		for _, filter := range filters {
			replaceLines = append(replaceLines, filter.AddLine(c.ifname))
		}
		if !reflect.DeepEqual(replaceLines, c.replaceLine) {
			t.Errorf("want %v, got %v", c.replaceLine, replaceLines)
		}
	}
}
