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

func TestUDPServer(t *testing.T) {
	const nMessages = 100

	l := log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr))
	s := syslogserver.NewUDPServer(l, syslogserver.UDPServerConfig{
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

	c, err := net.Dial("udp", s.Addr().String())
	require.NoError(t, err)

	for i := 0; i < nMessages; i++ {
		c.Write([]byte("<13>1 - - - - - " + strconv.Itoa(i) + " First"))
	}

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&received) == nMessages
	}, time.Second, time.Millisecond)
}
