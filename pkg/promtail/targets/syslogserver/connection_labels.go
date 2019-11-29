package syslogserver

import (
	"net"
	"strings"

	"github.com/prometheus/prometheus/pkg/labels"
)

func appendConnectionLabels(lb *labels.Builder, c net.Conn) labels.Labels {
	ip := ipFromConn(c).String()
	lb.Set("__syslog_connection_ip_address", ip)
	lb.Set("__syslog_connection_hostname", lookupAddr(ip))

	return lb.Labels()
}

func appendAddressLabels(lb *labels.Builder, a net.Addr) labels.Labels {
	ip := ipFromAddr(a).String()
	lb.Set("__syslog_connection_ip_address", ip)
	lb.Set("__syslog_connection_hostname", lookupAddr(ip))

	return lb.Labels()
}

func ipFromConn(c net.Conn) net.IP {
	return ipFromAddr(c.RemoteAddr())
}

func ipFromAddr(a net.Addr) net.IP {
	switch addr := a.(type) {
	case *net.TCPAddr:
		return addr.IP
	case *net.UDPAddr:
		return addr.IP
	}

	return nil
}

func lookupAddr(addr string) string {
	names, _ := net.LookupAddr(addr)
	return strings.Join(names, ",")
}
