package utils

import "testing"
import (
	"fmt"
	"strings"
)

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
