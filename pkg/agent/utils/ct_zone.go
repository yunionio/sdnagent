package utils

import (
	"errors"
	"hash/fnv"
)

type ZoneMan struct {
	zm   map[string]uint16
	zmr  map[uint16]string
	base uint16
}

func NewZoneMan(base uint16) *ZoneMan {
	return &ZoneMan{
		zm:   map[string]uint16{},
		zmr:  map[uint16]string{},
		base: base,
	}
}

func (zm *ZoneMan) AllocateZoneId(mac string) (uint16, error) {
	if i, ok := zm.zm[mac]; ok {
		return zm.base + i, nil
	}
	total := (1 << 16) - uint32(zm.base)
	if len(zm.zm) >= int(total) {
		return 0, errors.New("id depleted")
	}
	h := fnv.New32()
	h.Write([]byte(mac))
	i := uint16(h.Sum32() % total)
	j := i
	for {
		if _, ok := zm.zmr[i]; !ok {
			zm.zmr[i] = mac
			zm.zm[mac] = i
			return zm.base + i, nil
		}
		i += 1
		if i == j {
			break
		}
	}
	return 0, errors.New("error that never returns")
}

func (zm *ZoneMan) FreeZoneId(mac string) bool {
	if i, ok := zm.zm[mac]; ok {
		delete(zm.zm, mac)
		delete(zm.zmr, i)
		return true
	}
	return false
}
