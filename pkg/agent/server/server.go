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
	wg := &sync.WaitGroup{}
	ctx, cancelFunc := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, "wg", wg)
	theAgentServer = &AgentServer{
		flowMans:       map[string]*FlowMan{},
		flowMansLock:   &sync.RWMutex{},
		serversWatcher: watcher,
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
