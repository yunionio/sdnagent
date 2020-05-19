// Copyright 2019 Yunion
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"

	"yunion.io/x/sdnagent/pkg/agent/common"
	"yunion.io/x/sdnagent/pkg/agent/server"
	"yunion.io/x/sdnagent/pkg/agent/utils"
)

func LockPidFile(path string) (*os.File, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		return nil, err
	}
	pid := fmt.Sprintf("%d\n", os.Getpid())
	_, err = f.WriteString(pid)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func UnlockPidFile(f *os.File) error {
	defer f.Close()
	return os.Remove(f.Name())
}

func main() {
	var (
		ctx = context.Background()
		hc  *utils.HostConfig
		err error
	)
	if hc, err = utils.NewHostConfig(); err != nil {
		log.Errorln(errors.Wrap(err, "host config"))
	} else if err = hc.Auth(ctx); err != nil {
		log.Errorln(errors.Wrap(err, "keystone auth"))
	}

	{
		f, err := LockPidFile(hc.SdnPidFile)
		if err != nil {
			log.Errorf("create pid file %s failed: %s", hc.SdnPidFile, err)
			return
		}
		defer UnlockPidFile(f)
		err = os.Remove(common.UnixSocketFile)
		if err != nil && !os.IsNotExist(err) {
			log.Errorf("remove %s failed: %s", common.UnixSocketFile, err)
			return
		}
	}

	s := server.Server().HostConfig(hc)
	go hc.WatchChange(ctx, func() {
		log.Warningf("host config content changed")
		s.Stop()
	})
	go func() {
		sigChan := make(chan os.Signal)
		signal.Notify(sigChan, syscall.SIGINT)
		signal.Notify(sigChan, syscall.SIGTERM)
		sig := <-sigChan
		log.Infof("signal received: %s", sig)
		s.Stop()
	}()
	if err := s.Start(ctx); err != nil {
		log.Warningf("Start server error: %v", err)
	}
}
