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
	"sync"

	"yunion.io/x/sdnagent/pkg/agent/utils"
)

type AgentServer struct {
	once *sync.Once
	wg   *sync.WaitGroup

	flowMansLock *sync.RWMutex
	flowMans     map[string]*FlowMan

	ctx        context.Context
	ctxCancel  context.CancelFunc
	hostConfig *utils.HostConfig
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

func (s *AgentServer) HostConfig(hostConfig *utils.HostConfig) *AgentServer {
	s.hostConfig = hostConfig
	return s
}

func (s *AgentServer) Start(ctx context.Context) error {
	ctx = context.WithValue(ctx, "wg", s.wg)
	s.ctx, s.ctxCancel = context.WithCancel(ctx)

	defer s.wg.Wait()

	if s.hostConfig.SdnEnableGuestMan {
		watcher, err := newServersWatcher()
		if err != nil {
			panic("creating servers watcher failed: " + err.Error())
		}
		watcher.agent = s
		ifaceJanitor := newIfaceJanitor()

		s.wg.Add(2)
		go watcher.Start(s.ctx, s)
		go ifaceJanitor.Start(s.ctx)
	}

	if s.hostConfig.SdnEnableEipMan {
		eipMan := newEipMan(s)
		s.wg.Add(1)
		go eipMan.Start(s.ctx)
	}

	return nil
}

func (s *AgentServer) Stop() {
	s.once.Do(func() {
		s.ctxCancel()
	})
}

var theAgentServer *AgentServer

func init() {
	theAgentServer = &AgentServer{
		once: &sync.Once{},
		wg:   &sync.WaitGroup{},

		flowMans:     map[string]*FlowMan{},
		flowMansLock: &sync.RWMutex{},
	}
}

func Server() *AgentServer {
	return theAgentServer
}
