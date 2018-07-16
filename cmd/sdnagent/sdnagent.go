package main

import (
	"os"
	"os/signal"
	"syscall"

	"yunion.io/yunion-sdnagent/pkg/agent/server"
	"yunion.io/yunioncloud/pkg/log"
)

func main() {
	s := server.Server()
	go func() {
		sigChan := make(chan os.Signal)
		signal.Notify(sigChan, syscall.SIGINT)
		signal.Notify(sigChan, syscall.SIGTERM)
		sig := <-sigChan
		log.Infof("signal received: %s", sig)
		s.Stop()
	}()
	err := s.Start()
	if err != nil {
		log.Fatalf("Start server error: %v", err)
	}
}
