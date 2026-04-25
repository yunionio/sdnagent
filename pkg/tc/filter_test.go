package tc

import (
	"reflect"
	"testing"

	"yunion.io/x/jsonutils"
)

func TestParseFilterLines(t *testing.T) {
	parentQdisc := &QdiscHtb{
		SBaseTcQdisc: &SBaseTcQdisc{
			Kind:   "htb",
			Handle: "1:",
			Parent: "",
		},
		DefaultClass: 0,
	}
	cases := []struct {
		ifname      string
		in          []string
		want        []IFilter
		delLine     []string
		replaceLine []string
	}{
		{
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
						Parent:   parentQdisc,
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
	}
	for _, c := range cases {
		filters, err := parseFilterLines(c.in, []IQdisc{parentQdisc})
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
