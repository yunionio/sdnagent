package utils

import (
	"net"
)

type HostLocal struct {
	HostConfig *HostConfig
	Bridge     string
	Ifname     string
	IP         net.IP
	MAC        net.HardwareAddr
	MasterIP   net.IP
	MasterMAC  net.HardwareAddr
}
