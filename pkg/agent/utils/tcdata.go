package utils

import (
	"fmt"

	"yunion.io/yunion-sdnagent/pkg/tc"
)

type TcDataType string

const (
	TC_DATA_TYPE_HOSTLOCAL TcDataType = "hostlocal"
	TC_DATA_TYPE_GUEST     TcDataType = "guest"
)

type TcData struct {
	Type        TcDataType
	Ifname      string
	IngressMbps uint64
	EgressMbps  uint64
}

func (td *TcData) String() string {
	switch td.Type {
	case TC_DATA_TYPE_GUEST:
		return fmt.Sprintf("%s %s: ingress=%dMbps", td.Type, td.Ifname, td.IngressMbps)
	case TC_DATA_TYPE_HOSTLOCAL:
		return fmt.Sprintf("%s %s", td.Type, td.Ifname)
	}
	return ""
}

func (td *TcData) qdiscTreeGuest() (qt *tc.QdiscTree, err error) {
	if td.IngressMbps == 0 {
		qt, err = tc.NewQdiscTreeFromString("qdisc fq_codel root handle 1:")
	}
	bytesPerSec := td.IngressMbps * 1000 * 1000 / 8
	burst := bytesPerSec / 1000
	if burst < 3400 {
		burst = 3400
	}
	rates := tc.PrintRate(bytesPerSec)
	bursts := tc.PrintSize(burst)
	// tc accepts "mpu 64b" yet prints "mpu 0b" on qdisc show
	s := fmt.Sprintf("qdisc tbf root handle 1: rate %s burst %s latency 100ms\n", rates, bursts)
	s += "qdisc fq_codel parent 1: handle 10:\n"
	qt, err = tc.NewQdiscTreeFromString(s)
	return qt, err
}

func (td *TcData) qdiscTreeHostLocal() (qt *tc.QdiscTree, err error) {
	s := "qdisc fq_codel root handle 1:\n"
	qt, err = tc.NewQdiscTreeFromString(s)
	return qt, err
}

func (td *TcData) QdiscTree() (qt *tc.QdiscTree, err error) {
	switch td.Type {
	case TC_DATA_TYPE_GUEST:
		qt, err = td.qdiscTreeGuest()
	case TC_DATA_TYPE_HOSTLOCAL:
		qt, err = td.qdiscTreeHostLocal()
	}
	return
}
