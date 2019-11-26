package syslogserver

import (
	"fmt"
	"net"
	"runtime"

	"github.com/influxdata/go-syslog"
	"github.com/influxdata/go-syslog/rfc5424"
)

type UDPServer struct {
	messages chan *syslog.Result

	conn net.PacketConn
}

func (s *UDPServer) Start() error {
	s.messages = make(chan *syslog.Result)
	pc, err := net.ListenPacket("udp", "127.0.0.1:7777")
	if err != nil {
		return err
	}
	s.conn = pc

	// max udp package size
	buf := make([]byte, 65535)

	p := rfc5424.NewParser()

	for i := 0; i < runtime.NumCPU(); i++ {
		go func() {
			for {
				n, _, err := pc.ReadFrom(buf)
				if err != nil {
					fmt.Println("ReadFrom error:", err)
					return
				}
				msg, err := p.Parse(buf[:n])
				s.messages <- &syslog.Result{Message: msg, Error: err}
			}
		}()
	}

	return nil
}

func (s *UDPServer) Stop() error {
	err := s.conn.Close()
	close(s.messages)
	return err
}

func (s *UDPServer) Messages() <-chan *syslog.Result {
	return s.messages
}
