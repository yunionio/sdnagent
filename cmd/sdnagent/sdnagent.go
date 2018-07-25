package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"yunion.io/yunion-sdnagent/pkg/agent/common"
	"yunion.io/yunion-sdnagent/pkg/agent/server"
	"yunion.io/yunioncloud/pkg/log"
)

const (
	PIDFILE = "/var/run/yunion-sdnagent.pid"
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
	{
		f, err := LockPidFile(PIDFILE)
		if err != nil {
			log.Errorf("create pid file %s failed: %s", PIDFILE, err)
			return
		}
		defer UnlockPidFile(f)
		err = os.Remove(common.UnixSocketFile)
		if err != nil && !os.IsNotExist(err) {
			log.Errorf("remove %s failed: %s", common.UnixSocketFile, err)
			return
		}
	}

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
