package syslogserver

import (
	"net"
	"runtime"
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/influxdata/go-syslog/rfc5424"
	"github.com/prometheus/prometheus/pkg/labels"
)

type UDPServerConfig struct {
	// ListenAddress is the address to listen on for syslog messages.
	ListenAddress string `yaml:"listen_address"`
}

type UDPServer struct {
	config UDPServerConfig
	logger log.Logger

	conn     net.PacketConn
	messages chan *Message

	done *sync.WaitGroup
}

func NewUDPServer(l log.Logger, conf UDPServerConfig) SyslogServer {
	return &UDPServer{
		logger: l,
		config: conf,

		messages: make(chan *Message),
		done:     new(sync.WaitGroup),
	}
}

func (s *UDPServer) Start() error {
	pc, err := net.ListenPacket("udp", s.config.ListenAddress)
	if err != nil {
		return err
	}
	s.conn = pc

	receivers := runtime.NumCPU()
	s.done.Add(receivers)
	for i := 0; i < receivers; i++ {
		go func() {
			defer s.done.Done()
			// max udp package size
			buf := make([]byte, 65535)

			p := rfc5424.NewParser()
			for {
				n, addr, err := s.conn.ReadFrom(buf)
				if err != nil {
					return
				}
				msg, err := p.Parse(buf[:n])
				s.messages <- &Message{
					Message: msg,
					Error:   err,
					Labels:  appendAddressLabels(labels.NewBuilder(nil), addr),
				}
			}
		}()
	}

	return nil
}

func (s *UDPServer) Stop() error {
	err := s.conn.Close()
	s.done.Wait()
	close(s.messages)
	return err
}

func (s *UDPServer) Messages() <-chan *Message {
	return s.messages
}

func (s *UDPServer) Addr() net.Addr {
	return s.conn.LocalAddr()
}
