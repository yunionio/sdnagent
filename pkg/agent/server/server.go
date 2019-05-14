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
	"yunion.io/x/sdnagent/pkg/agent/common"
	pb "yunion.io/x/sdnagent/pkg/agent/proto"
)

type AgentServer struct {
	rpcServer      *grpc.Server
	ctx            context.Context
	ctxCancel      context.CancelFunc
	wg             *sync.WaitGroup
	serversWatcher *serversWatcher
	flowMansLock   *sync.RWMutex
	flowMans       map[string]*FlowMan
	ifaceJanitor   *ifaceJanitor
}

func (s *AgentServer) GetFlowMan(bridge string) *FlowMan {
	theAgentServer.flowMansLock.RLock()
	defer theAgentServer.flowMansLock.RUnlock()
	if flowman, ok := theAgentServer.flowMans[bridge]; ok {
		return flowman
	}
	flowman := NewFlowMan(bridge)
	theAgentServer.flowMans[bridge] = flowman
	go flowman.Start(s.ctx)
	s.wg.Add(1)
	return flowman
}

func (s *AgentServer) Start() error {
	lis, err := net.Listen("unix", common.UnixSocketFile)
	if err != nil {
		log.Fatalf("listen %s failed: %s", common.UnixSocketFile, err)
	}
	defer lis.Close()
	defer s.wg.Wait()
	go s.serversWatcher.Start(s.ctx, s)
	go s.ifaceJanitor.Start(s.ctx)
	s.wg.Add(2)
	err = s.rpcServer.Serve(lis)
	return err
}

func (s *AgentServer) Stop() {
	s.rpcServer.GracefulStop()
	s.ctxCancel()
}

var theAgentServer *AgentServer

func init() {
	watcher, err := newServersWatcher()
	if err != nil {
		panic("creating servers watcher failed: " + err.Error())
	}
	ifaceJanitor := newIfaceJanitor()
	wg := &sync.WaitGroup{}
	ctx, cancelFunc := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, "wg", wg)
	theAgentServer = &AgentServer{
		flowMans:       map[string]*FlowMan{},
		flowMansLock:   &sync.RWMutex{},
		serversWatcher: watcher,
		ifaceJanitor:   ifaceJanitor,
		wg:             wg,
		ctx:            ctx,
		ctxCancel:      cancelFunc,
	}
	watcher.agent = theAgentServer
	vSwitchService := newVSwitchService(theAgentServer)
	openflowService := newOpenflowService(theAgentServer)
	s := grpc.NewServer()
	pb.RegisterVSwitchServer(s, vSwitchService)
	pb.RegisterOpenflowServer(s, openflowService)
	reflection.Register(s)
	theAgentServer.rpcServer = s
}

func Server() *AgentServer {
	return theAgentServer
}
