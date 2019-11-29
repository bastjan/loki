package syslogserver

import (
	"github.com/influxdata/go-syslog"
	"github.com/prometheus/prometheus/pkg/labels"
	"net"
)

type Message struct {
	Labels  labels.Labels
	Message syslog.Message
	Error   error
}

type SyslogServer interface {
	Start() error
	Stop() error
	Messages() <-chan *Message
	Addr() net.Addr
}
