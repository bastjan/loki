package syslogserver_test

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/grafana/loki/pkg/promtail/targets/syslogserver"
	"github.com/stretchr/testify/require"
)

const nMessages = 100

func TestUDPServer(t *testing.T) {
	l := log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr))
	s := syslogserver.NewUDPServer(l, syslogserver.UDPServerConfig{
		ListenAddress: "127.0.0.1:0",
	})

	require.NoError(t, s.Start())

	go func() {
		var r int
		for range s.Messages() {
			r++
		}
		fmt.Println("Messages: ", r)
	}()

	c, err := net.Dial("udp", s.Addr().String())
	require.NoError(t, err)

	for i := 0; i < nMessages; i++ {
		c.Write([]byte("<13>1 - - - - - " + strconv.Itoa(i) + " First"))
	}

	time.Sleep(5 * time.Second)
	s.Stop()
	time.Sleep(time.Second)
	t.Fail()
}
