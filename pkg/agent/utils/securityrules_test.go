package utils

import "testing"
import (
	"fmt"
	"reflect"
	"strings"
)

func TestSecurityRuleOvsMatches(t *testing.T) {
	cases := []struct {
		in      string
		matches []string
	}{
		{
			in: `in:allow tcp 3389`,
			matches: []string{
				`tcp,tp_dst=3389`,
			},
		},
		{
			in: `in:allow tcp 3389-3389`,
			matches: []string{
				`tcp,tp_dst=3389`,
			},
		},
		{
			in: `in:allow tcp 22-3389`,
			matches: []string{
				`tcp,tp_dst=0x16/0xfffe`,
				`tcp,tp_dst=0x18/0xfff8`,
				`tcp,tp_dst=0x20/0xffe0`,
				`tcp,tp_dst=0x40/0xffc0`,
				`tcp,tp_dst=0x80/0xff80`,
				`tcp,tp_dst=0x100/0xff00`,
				`tcp,tp_dst=0x200/0xfe00`,
				`tcp,tp_dst=0x400/0xfc00`,
				`tcp,tp_dst=0x800/0xfc00`,
				`tcp,tp_dst=0xc00/0xff00`,
				`tcp,tp_dst=0xd00/0xffe0`,
				`tcp,tp_dst=0xd20/0xfff0`,
				`tcp,tp_dst=0xd30/0xfff8`,
				`tcp,tp_dst=0xd38/0xfffc`,
				`tcp,tp_dst=0xd3c/0xfffe`,
			},
		},
		{
			in: `in:allow tcp 22,3389`,
			matches: []string{
				`tcp,tp_dst=22`,
				`tcp,tp_dst=3389`,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			sr, err := NewSecurityRule(c.in)
			if err != nil {
				t.Fatalf("unexpected err: %s", err)
			}
			got := sr.OvsMatches()
			if !reflect.DeepEqual(c.matches, got) {
				t.Errorf("ovs matches, want %d, got %d;\n%s\n--\n%s",
					len(c.matches), len(got),
					"  "+strings.Join(c.matches, "\n  "),
					"  "+strings.Join(got, "\n  "),
				)
			}
		})
	}
}

func TestPortRangeToMasks(t *testing.T) {
	cases := []struct {
		s uint16
		e uint16
	}{
		{s: 0, e: 0},
		{s: 0, e: 1},
		{s: 0, e: 100},
		{s: 0, e: 103},
		{s: 0, e: 32},
		{s: 0, e: 65535},
		{s: 1, e: 1},
		{s: 1, e: 100},
		{s: 1, e: 32},
		{s: 1, e: 103},
		{s: 1, e: 65535},
		{s: 80, e: 80},
		{s: 80, e: 443},
		{s: 81, e: 81},
		{s: 81, e: 440},
		{s: 81, e: 443},
	}
	maskToString := func(m [2]uint16) string {
		return fmt.Sprintf("%016b/%016b", m[0], m[1])
	}
	masksToString := func(ms [][2]uint16) []string {
		r := []string{}
		for _, m := range ms {
			r = append(r, maskToString(m))
		}
		return r
	}
	for _, c := range cases {
		fmt.Printf("## %d-%d\n", c.s, c.e)
		fmt.Printf("  %016b\n", c.s)
		fmt.Printf("  %016b\n", c.e)
		ms := PortRangeToMasks(c.s, c.e)
		fmt.Printf("  %s\n", strings.Join(masksToString(ms), "\n  "))
		// before range
		for i := uint16(0); i < c.s; i++ {
			mc := 0
			for _, m := range ms {
				if m[0] == i&m[1] {
					mc += 1
				}
			}
			if mc != 0 {
				t.Errorf("bad: port %d matches %d times", i, mc)
			}
		}
		// after range
		if c.e < 65535 {
			for i := c.e + 1; ; i++ {
				mc := 0
				for _, m := range ms {
					if m[0] == i&m[1] {
						mc += 1
					}
				}
				if i == 65535 {
					if mc != 0 {
						t.Errorf("bad: port %d matches %d times", i, mc)
					}
					break
				}
			}
		}
		// in range
		for i := c.s; ; i++ {
			mc := 0
			for _, m := range ms {
				if m[0] == i&m[1] {
					mc += 1
				}
			}
			if i == c.e {
				if mc != 1 {
					t.Errorf("bad: port %d matches %d times", i, mc)
				}
				break
			}
		}
	}
}
