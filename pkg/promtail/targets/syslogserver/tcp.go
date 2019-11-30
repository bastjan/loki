package syslogserver

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/grafana/loki/pkg/promtail/targets/syslogparser"
	"github.com/mwitkow/go-conntrack"
	"github.com/prometheus/prometheus/pkg/labels"
)

const defaultIdleTimeout = 120 * time.Second

type TCPServerConfig struct {
	// ListenAddress is the address to listen on for syslog messages.
	ListenAddress string `yaml:"listen_address"`

	// IdleTimeout is the idle timeout for tcp connections.
	IdleTimeout time.Duration `yaml:"idle_timeout"`
}

type TCPServer struct {
	config TCPServerConfig
	logger log.Logger

	listener net.Listener
	messages chan *Message

	shutdown          chan struct{}
	connectionsClosed *sync.WaitGroup
}

// NewTCPServer creates a new TCP syslog server.
func NewTCPServer(l log.Logger, conf TCPServerConfig) SyslogServer {
	return &TCPServer{
		logger:            l,
		config:            conf,
		messages:          make(chan *Message),
		shutdown:          make(chan struct{}),
		connectionsClosed: new(sync.WaitGroup),
	}
}

// Start starts the server.
func (s *TCPServer) Start() error {
	return s.run()
}

// Stop stops the server and closes the message channel.
func (s *TCPServer) Stop() error {
	close(s.shutdown)
	err := s.listener.Close()
	s.connectionsClosed.Wait()
	close(s.messages)
	return err
}

// Messages returns the messages channel.
func (s *TCPServer) Messages() <-chan *Message {
	return s.messages
}

// Addr returns the address the server is listening on.
func (s *TCPServer) Addr() net.Addr {
	return s.listener.Addr()
}

func (s *TCPServer) run() error {
	l, err := net.Listen("tcp", s.config.ListenAddress)
	l = conntrack.NewListener(l, conntrack.TrackWithName("syslog_target/"+s.config.ListenAddress))
	if err != nil {
		return fmt.Errorf("error setting up tcp listener %w", err)
	}
	s.listener = l
	level.Info(s.logger).Log("msg", "syslog listening on address", "address", s.listener.Addr().String())

	s.connectionsClosed.Add(1)
	go s.acceptConnections()

	return nil
}

func (s *TCPServer) acceptConnections() {
	defer s.connectionsClosed.Done()

	l := log.With(s.logger, "address", s.listener.Addr().String())

	for {
		c, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				level.Info(l).Log("msg", "syslog server shutting down")
				return
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				backoff := 10 * time.Millisecond
				level.Warn(l).Log("msg", "failed to accept syslog connection", "err", err, "retry_in", backoff)
				time.Sleep(backoff)
				continue
			}

			level.Error(l).Log("msg", "failed to accept syslog connection. quiting", "err", err)
			return
		}

		s.connectionsClosed.Add(1)
		go s.handleConnection(c)
	}
}

func (s *TCPServer) handleConnection(cn net.Conn) {
	defer s.connectionsClosed.Done()

	c := &idleTimeoutConn{cn, s.idleTimeout()}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-done:
		case <-s.shutdown:
		}
		c.Close()
	}()

	connLabels := appendConnectionLabels(labels.NewBuilder(nil), c)

	for msg := range syslogparser.ParseStream(c) {
		s.messages <- &Message{
			Labels:  connLabels,
			Message: msg.Message,
			Error:   msg.Error,
		}
	}
}

func (s *TCPServer) idleTimeout() time.Duration {
	if tm := s.config.IdleTimeout; tm != 0 {
		return tm
	}
	return defaultIdleTimeout
}
