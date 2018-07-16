package server

import (
	"time"
)

const (
	DefaultHostConfigPath    string        = "/etc/yunion/host.conf"
	FlowManIdleCheckDuration time.Duration = 5 * time.Second
	WatcherRefreshRate       time.Duration = 7 * time.Second
)
