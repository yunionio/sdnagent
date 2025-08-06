package utils

import (
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type Route struct {
	Net     net.IPNet
	Gateway net.IP
	Dev     string
	Metric  int
}

func (r *Route) String() string {
	s := r.Net.String()
	if r.Gateway != nil {
		if !r.Gateway.IsUnspecified() {
			s += " via " + r.Gateway.String()
		}
	}
	if r.Dev != "" {
		s += " dev " + r.Dev
	}
	if r.Metric > 0 {
		s += " metric " + strconv.FormatInt(int64(r.Metric), 10)
	}
	return s
}

type Routes []Route

func parseRoutes(s string) (Routes, error) {
	// Iface   Destination     Gateway         Flags   RefCnt  Use     Metric  Mask            MTU     Window  IRTT
	// br0     00000000        01DEA80A        0003    0       0       0       00000000        0       0       0
	lines := strings.Split(s, "\n")
	lines = lines[1:]
	routes := []Route{}
	for _, line := range lines {
		elms := strings.Split(line, "\t")
		if len(elms) < 8 {
			continue
		}
		ipint, err := strconv.ParseUint(elms[1], 16, 32)
		if err != nil {
			continue
		}
		gwint, err := strconv.ParseUint(elms[2], 16, 32)
		if err != nil {
			continue
		}
		mask, err := strconv.ParseUint(elms[7], 16, 32)
		if err != nil {
			continue
		}
		metric, err := strconv.ParseUint(elms[6], 10, 32)
		if err != nil {
			continue
		}

		toIP := func(i uint64) net.IP {
			return net.IPv4(
				byte(i&0xff),
				byte((i>>8)&0xff),
				byte((i>>16)&0xff),
				byte(i>>24),
			)
		}
		routes = append(routes, Route{
			Net: net.IPNet{
				IP:   toIP(ipint),
				Mask: net.IPMask(toIP(mask).To4()),
			},
			Gateway: toIP(gwint),
			Dev:     elms[0],
			Metric:  int(metric),
		})
	}
	return routes, nil
}

func GetRoutes() (Routes, error) {
	d, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return nil, err
	}
	return parseRoutes(string(d))
}

func (rs Routes) String() string {
	lines := make([]string, 0, len(rs))
	for i := range rs {
		lines = append(lines, rs[i].String())
	}
	return strings.Join(lines, "\n")
}

func (rs Routes) Lookup(ipstr string) (Route, error) {
	ip := net.ParseIP(ipstr)

	ri := -1
	pl := -1
	metric := 10000000
	for i, r := range rs {
		pl1, _ := r.Net.Mask.Size()
		if pl > pl1 {
			continue
		}
		if r.Net.Contains(ip) {
			metric1 := r.Metric
			if pl < pl1 {
				ri = i
				pl = pl1
				metric = metric1
				continue
			}
			// pl == pl1
			if metric > metric1 {
				ri = i
				metric = metric1
			}
		}
	}
	if ri >= 0 {
		return rs[ri], nil
	}
	return Route{}, errors.Errorf("no route to %s", ipstr)
}

func RouteLookup(ipstr string) (Route, error) {
	routes, err := GetRoutes()
	if err != nil {
		return Route{}, errors.Wrap(err, "get routes")
	}
	rt, err := routes.Lookup(ipstr)
	if err != nil {
		return Route{}, errors.Wrapf(err, "lookup route %s", ipstr)
	}
	return rt, nil
}
