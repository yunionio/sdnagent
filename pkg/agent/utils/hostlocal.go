package utils

import (
	"net"
)

type HostLocal struct {
	MetadataPort int
	IP           net.IP
	MAC          net.HardwareAddr
	K8SCidr      *net.IPNet
	Bridge       string
	Ifname       string
}
