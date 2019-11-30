package syslogserver_test

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"

	"github.com/grafana/loki/pkg/promtail/targets/syslogserver"
)

func TestTCPServer(t *testing.T) {
	const nMessages = 100

	l := log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr))
	s := syslogserver.NewTCPServer(l, syslogserver.TCPServerConfig{
		ListenAddress: "127.0.0.1:0",
	})

	require.NoError(t, s.Start())
	defer s.Stop()

	var received int32
	go func() {
		for range s.Messages() {
			atomic.AddInt32(&received, 1)
		}
		fmt.Println("Messages: ", received)
	}()

	c, err := net.Dial("tcp", s.Addr().String())
	require.NoError(t, err)

	for i := 0; i < nMessages; i++ {
		c.Write([]byte("<13>1 - - - - - " + strconv.Itoa(i) + " First\n"))
	}

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&received) == nMessages
	}, time.Second, time.Millisecond)
}

func TestSyslogTarget_InvalidData(t *testing.T) {
	l := log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr))
	s := syslogserver.NewTCPServer(l, syslogserver.TCPServerConfig{
		ListenAddress: "127.0.0.1:0",
	})

	require.NoError(t, s.Start())
	defer s.Stop()
	go consumeMessages(s)

	addr := s.Addr().String()
	c, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer c.Close()

	_, err = fmt.Fprint(c, "xxx")

	// syslog target should immediately close the connection if sent invalid data
	c.SetDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	_, err = c.Read(buf)
	require.EqualError(t, err, "EOF")
}

func TestSyslogTarget_IdleTimeout(t *testing.T) {
	l := log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr))
	s := syslogserver.NewTCPServer(l, syslogserver.TCPServerConfig{
		ListenAddress: "127.0.0.1:0",
		IdleTimeout:   50 * time.Millisecond,
	})

	require.NoError(t, s.Start())
	defer s.Stop()
	go consumeMessages(s)

	addr := s.Addr().String()
	c, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	defer c.Close()

	// connection should be closed before the higher timeout
	// from SetDeadline fires
	c.SetDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1)
	_, err = c.Read(buf)
	require.EqualError(t, err, "EOF")
}

func consumeMessages(s syslogserver.SyslogServer) {
	for range s.Messages() {
	}
}
