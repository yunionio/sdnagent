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

package server

import (
	"context"
	"net"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"yunion.io/x/log"
	"yunion.io/x/pkg/errors"
	"yunion.io/x/sdnagent/pkg/agent/common"
	pb "yunion.io/x/sdnagent/pkg/agent/proto"
	"yunion.io/x/sdnagent/pkg/agent/utils"
)

type AgentServer struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	once      *sync.Once
	wg        *sync.WaitGroup

	flowMansLock *sync.RWMutex
	flowMans     map[string]*FlowMan

	hostConfig *utils.HostConfig

	rpcServer *grpc.Server
}

func (s *AgentServer) GetFlowMan(bridge string) *FlowMan {
	theAgentServer.flowMansLock.RLock()
	defer theAgentServer.flowMansLock.RUnlock()
	if flowman, ok := theAgentServer.flowMans[bridge]; ok {
		return flowman
	}
	flowman := NewFlowMan(bridge)
	theAgentServer.flowMans[bridge] = flowman
	s.wg.Add(1)
	go flowman.Start(s.ctx)
	return flowman
}

func (s *AgentServer) Start() error {
	var (
		hc  *utils.HostConfig
		err error
	)
	if hc, err = utils.NewHostConfig(); err != nil {
		return errors.Wrap(err, "host config")
	} else if err = hc.Auth(s.ctx); err != nil {
		return errors.Wrap(err, "keystone auth")
	} else {
		s.hostConfig = hc
		go hc.WatchChange(s.ctx, func() {
			log.Warningf("host config content changed")
			s.Stop()
		})
	}

	defer s.wg.Wait()

	if hc.SdnEnableGuestMan {
		watcher, err := newServersWatcher()
		if err != nil {
			panic("creating servers watcher failed: " + err.Error())
		}
		watcher.agent = s
		ifaceJanitor := newIfaceJanitor()

		vSwitchService := newVSwitchService(s)
		openflowService := newOpenflowService(s)
		rpcServer := grpc.NewServer()
		pb.RegisterVSwitchServer(rpcServer, vSwitchService)
		pb.RegisterOpenflowServer(rpcServer, openflowService)
		reflection.Register(rpcServer)
		s.rpcServer = rpcServer

		lis, err := net.Listen("unix", common.UnixSocketFile)
		if err != nil {
			log.Fatalf("listen %s failed: %s", common.UnixSocketFile, err)
		}
		defer lis.Close()

		s.wg.Add(2)
		go watcher.Start(s.ctx, s)
		go ifaceJanitor.Start(s.ctx)
		go func() {
			err := rpcServer.Serve(lis)
			if err != nil {
				log.Warningf("rpc server serve returned: %v", err)
			}
		}()
	}

	return nil
}

func (s *AgentServer) Stop() {
	s.once.Do(func() {
		if s.rpcServer != nil {
			s.rpcServer.GracefulStop()
		}
		s.ctxCancel()
	})
}

var theAgentServer *AgentServer

func init() {
	wg := &sync.WaitGroup{}
	ctx, cancelFunc := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, "wg", wg)
	theAgentServer = &AgentServer{
		ctx:       ctx,
		ctxCancel: cancelFunc,
		once:      &sync.Once{},
		wg:        wg,

		flowMans:     map[string]*FlowMan{},
		flowMansLock: &sync.RWMutex{},
	}
}

func Server() *AgentServer {
	return theAgentServer
}
