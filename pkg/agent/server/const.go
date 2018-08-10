package server

import (
	"time"
)

const (
	DefaultHostConfigPath     string        = "/etc/yunion/host.conf"
	GuestCtZoneBase           uint16        = 1000
	FlowManIdleCheckDuration  time.Duration = 13 * time.Second
	TcManIdleCheckDuration    time.Duration = 17 * time.Second
	WatcherRefreshRate        time.Duration = 31 * time.Second
	WatcherRefreshRateOnError time.Duration = 3 * time.Second
	WatcherRecentPendingTime  time.Duration = WatcherRefreshRateOnError * 5
)
