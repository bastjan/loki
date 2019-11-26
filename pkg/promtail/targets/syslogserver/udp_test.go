package syslogserver_test

import (
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/grafana/loki/pkg/promtail/targets/syslogserver"
	"github.com/stretchr/testify/require"
)

const nMessages = 100_000

func TestUDPServer(t *testing.T) {
	s := new(syslogserver.UDPServer)

	require.NoError(t, s.Start())

	go func() {
		var r int
		for range s.Messages() {
			r++
		}
		fmt.Println("Messages: ", r)
	}()

	c, err := net.Dial("udp", "127.0.0.1:7777")
	require.NoError(t, err)

	for i := 0; i < nMessages; i++ {
		c.Write([]byte("<13>1 - - - - - " + strconv.Itoa(i) + " First"))
	}

	time.Sleep(5 * time.Second)
	s.Stop()
	time.Sleep(time.Second)
	t.Fail()
}
