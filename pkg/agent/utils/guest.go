package utils

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

type guestDesc struct {
	NICs               []*guestNIC `json:"nics"`
	SecurityRules      string      `json:"security_rules"`
	AdminSecurityRules string      `json:"admin_security_rules"`
	Secgroup           string
	Name               string
}

type guestNIC struct {
	Bridge     string
	Bw         int
	Dns        string
	Domain     string
	Driver     string
	Gateway    string
	IfnameHost string `json:"ifname"`
	Index      int
	IfnameVM   string `json:"interface"`
	IP         string `json:"ip"`
	MAC        string
	Masklen    int
	Net        string
	NetId      string `json:"net_id"`
	Virtual    bool
	VLAN       int
	WireId     string `json:"wire_id"`

	CtZoneId uint16 `json:"-"`
	PortNo   int    `json:"-"`
}

func (n *guestNIC) Map() map[string]interface{} {
	m := map[string]interface{}{
		"IP":      n.IP,
		"MAC":     n.MAC,
		"VLAN":    n.VLAN & 0xfff,
		"CT_ZONE": n.CtZoneId,
		"PortNo":  n.PortNo,
	}
	vlanTci := n.VLAN & 0xfff
	if n.VLAN > 1 {
		// 802.1Q vlan header present
		vlanTci |= 0x1000
	} else {
		vlanTci = 0
	}
	m["VLANTci"] = fmt.Sprintf("0x%04x/0x1fff", vlanTci)
	return m
}

type Guest struct {
	Id            string
	Path          string
	Name          string
	SecurityRules *SecurityRules
	NICs          []*guestNIC
	HostConfig    *HostConfig
}

func (g *Guest) IsVM() bool {
	startvmPath := path.Join(g.Path, "startvm")
	_, err := os.Stat(startvmPath)
	if err == nil {
		return true
	}
	return false
}

func (g *Guest) Running() bool {
	pidPath := path.Join(g.Path, "pid")
	pidData, err := ioutil.ReadFile(pidPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		// NOTE just to be reservative
		return true
	}
	if len(pidData) == 0 {
		return false
	}
	// NOTE check /proc/<pid>/exe links a qemu executable
	pidStr := strings.TrimSpace(string(pidData))
	procPath := path.Join("/proc", pidStr)
	fileInfo, err := os.Stat(procPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		// NOTE just to be reservative
		return true
	}
	if fileInfo.IsDir() {
		return true
	}
	return false
}

func (g *Guest) LoadDesc() error {
	descPath := path.Join(g.Path, "desc")
	descFile, err := os.Open(descPath)
	if err != nil {
		return err
	}
	defer descFile.Close()
	dec := json.NewDecoder(descFile)
	var desc guestDesc
	err = dec.Decode(&desc)
	if err != nil {
		return err
	}
	g.Name = desc.Name
	g.NICs = desc.NICs
	{
		if len(desc.Secgroup) == 0 {
			desc.SecurityRules = "out:allow any; in:allow any"
		}
		rstr := desc.AdminSecurityRules + "; " + desc.SecurityRules
		rs, err := NewSecurityRules(rstr)
		if err != nil {
			return err
		}
		g.SecurityRules = rs
	}
	return nil
}
