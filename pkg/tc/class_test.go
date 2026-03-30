package tc

import (
	"reflect"
	"testing"

	"yunion.io/x/jsonutils"
)

func TestParseClassLines(t *testing.T) {
	rootHtbClass := &SHtbClass{
		SBaseTcClass: &SBaseTcClass{
			Kind:    "htb",
			ClassId: "1:1",
			Parent:  nil,
		},
		Rate: 10000000000,
		Ceil: 10000000000,
	}
	cases := []struct {
		ifname      string
		in          []string
		want        []IClass
		delLine     []string
		replaceLine []string
	}{
		{
			ifname: "eth0",
			in: []string{
				"class htb 1:2 parent 1:1 prio 0 rate 1Gbit ceil 10Gbit burst 1375b cburst 0b",
				"class htb 1:3 parent 1:1 prio 0 rate 100Mbit ceil 100Mbit burst 1600b cburst 1600b",
				"class htb 1:1 root rate 10Gbit ceil 10Gbit burst 0b cburst 0b",
			},
			want: []IClass{
				rootHtbClass,
				&SHtbClass{
					SBaseTcClass: &SBaseTcClass{
						Kind:    "htb",
						ClassId: "1:2",
						Parent:  rootHtbClass,
					},
					Rate: 1000000000,
					Ceil: 10000000000,
				},
				&SHtbClass{
					SBaseTcClass: &SBaseTcClass{
						Kind:    "htb",
						ClassId: "1:3",
						Parent:  rootHtbClass,
					},
					Rate: 100000000,
					Ceil: 100000000,
				},
			},
			delLine: []string{
				"class delete dev eth0 parent 1: classid 1:1 htb rate 10Gbit ceil 10Gbit",
				"class delete dev eth0 parent 1:1 classid 1:2 htb rate 1Gbit ceil 10Gbit",
				"class delete dev eth0 parent 1:1 classid 1:3 htb rate 100Mbit ceil 100Mbit",
			},
			replaceLine: []string{
				"class add dev eth0 parent 1: classid 1:1 htb rate 10Gbit ceil 10Gbit",
				"class add dev eth0 parent 1:1 classid 1:2 htb rate 1Gbit ceil 10Gbit",
				"class add dev eth0 parent 1:1 classid 1:3 htb rate 100Mbit ceil 100Mbit",
			},
		},
	}
	for _, c := range cases {
		got, err := parseClassLines(c.in)
		if err != nil {
			t.Fatalf("parse class lines: %v", err)
		}
		if len(got) != len(c.want) {
			t.Fatalf("want %d classes, got %d", len(c.want), len(got))
			continue
		}
		for i := range got {
			if !got[i].Equals(c.want[i]) {
				t.Fatalf("class %d: want %s, got %s", i, jsonutils.Marshal(c.want[i]), jsonutils.Marshal(got[i]))
			}
		}
		delLines := []string{}
		for _, cls := range got {
			delLines = append(delLines, cls.DeleteLine(c.ifname))
		}
		if !reflect.DeepEqual(delLines, c.delLine) {
			t.Fatalf("want %v, got %v", c.delLine, delLines)
		}
		replaceLines := []string{}
		for _, cls := range got {
			replaceLines = append(replaceLines, cls.AddLine(c.ifname))
		}
		if !reflect.DeepEqual(replaceLines, c.replaceLine) {
			t.Fatalf("want %v, got %v", c.replaceLine, replaceLines)
		}
	}
}

func TestCompareBase(t *testing.T) {
	cases := []struct {
		a    IClass
		b    IClass
		want int
	}{
		{
			a: &SHtbClass{
				SBaseTcClass: &SBaseTcClass{
					Kind:    "htb",
					ClassId: "1:3",
					Parent:  nil,
				},
				Rate: 10000000,
				Ceil: 10000000,
			},
			b: &SHtbClass{
				SBaseTcClass: &SBaseTcClass{
					Kind:    "htb",
					ClassId: "1:3",
					Parent:  nil,
				},
				Rate: 10000000,
				Ceil: 10000000,
			},
			want: 0,
		},
		{
			a: &SHtbClass{
				SBaseTcClass: &SBaseTcClass{
					Kind:    "htb",
					ClassId: "1:3",
					Parent:  nil,
				},
				Rate: 10000000,
				Ceil: 10000000,
			},
			b: &SHtbClass{
				SBaseTcClass: &SBaseTcClass{
					Kind:    "htb",
					ClassId: "1:3",
					Parent:  nil,
				},
				Rate: 20000000,
				Ceil: 20000000,
			},
			want: 0,
		},
		{
			a: &SHtbClass{
				SBaseTcClass: &SBaseTcClass{
					Kind:    "htb",
					ClassId: "1:2",
					Parent:  nil,
				},
				Rate: 10000000,
				Ceil: 10000000,
			},
			b: &SHtbClass{
				SBaseTcClass: &SBaseTcClass{
					Kind:    "htb",
					ClassId: "1:100",
					Parent:  nil,
				},
				Rate: 20000000,
				Ceil: 20000000,
			},
			want: -1,
		},
	}
	for i, c := range cases {
		got := c.a.CompareBase(c.b)
		if got != c.want {
			t.Fatalf("[case %d] compare %s and %s, want %d, got %d", i, c.a.Id(), c.b.Id(), c.want, got)
		}
	}
}
