package utils

import "testing"
import (
	"fmt"
)

func TestCtZone(t *testing.T) {
	base := 1000
	start := 382
	end := 0x10000 + 382 - base
	man := NewZoneMan(uint16(base))
	mm := map[uint16]string{}
	for i := start; i < end; i++ {
		m := fmt.Sprintf("%d", i)
		j, err := man.AllocateZoneId(m)
		if err != nil {
			t.Errorf("err: %s", err)
			return
		}
		if om, ok := mm[j]; ok {
			t.Errorf("err: dup id %d from %s, %s", j, m, om)
			return
		}
		mm[j] = m
	}
	j, err := man.AllocateZoneId("xxoo")
	if err == nil {
		t.Errorf("should exhausted, got %d", j)
		return
	}
	for i := start; i < end; i++ {
		m := fmt.Sprintf("%d", i)
		b := man.FreeZoneId(m)
		if !b {
			t.Errorf("should be there: %s", m)
			return
		}
		b = man.FreeZoneId(m)
		if b {
			t.Errorf("should not be there: %s", m)
			return
		}
	}
}
