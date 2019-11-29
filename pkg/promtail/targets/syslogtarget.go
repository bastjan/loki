package targets

import (
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/influxdata/go-syslog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/relabel"

	"github.com/grafana/loki/pkg/promtail/api"
	"github.com/grafana/loki/pkg/promtail/scrape"
	"github.com/grafana/loki/pkg/promtail/targets/syslogserver"
)

var (
	syslogEntries = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "promtail",
		Name:      "syslog_target_entries_total",
		Help:      "Total number of successful entries sent to the syslog target",
	})
	syslogParsingErrors = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "promtail",
		Name:      "syslog_target_parsing_errors_total",
		Help:      "Total number of parsing errors while receiving syslog messages",
	})

	defaultIdleTimeout = 120 * time.Second
)

// SyslogTarget listens to syslog messages.
type SyslogTarget struct {
	logger        log.Logger
	handler       api.EntryHandler
	config        *scrape.SyslogTargetConfig
	relabelConfig []*relabel.Config

	server   syslogserver.SyslogServer
	messages chan message

	shutdown *sync.WaitGroup
}

type message struct {
	labels  model.LabelSet
	message string
}

// NewSyslogTarget configures a new SyslogTarget.
func NewSyslogTarget(
	logger log.Logger,
	handler api.EntryHandler,
	relabel []*relabel.Config,
	config *scrape.SyslogTargetConfig,
) (*SyslogTarget, error) {

	t := &SyslogTarget{
		logger:        logger,
		handler:       handler,
		config:        config,
		relabelConfig: relabel,

		shutdown: new(sync.WaitGroup),
	}

	t.messages = make(chan message)
	go t.messageSender()

	err := t.run()
	return t, err
}

func (t *SyslogTarget) run() error {
	t.server = syslogserver.NewTCPServer(t.logger, syslogserver.TCPServerConfig{
		ListenAddress: t.config.ListenAddress,
		IdleTimeout:   t.config.IdleTimeout,
	})

	err := t.server.Start()
	if err != nil {
		return err
	}

	t.shutdown.Add(1)
	go func() {
		defer t.shutdown.Done()

		for msg := range t.server.Messages() {
			if msg.Error != nil {
				t.handleMessageError(msg.Error)
				continue
			}
			t.handleMessage(msg.Labels, msg.Message)
		}
	}()

	return nil
}

func (t *SyslogTarget) handleMessageError(err error) {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		level.Debug(t.logger).Log("msg", "connection timed out", "err", ne)
		return
	}
	level.Debug(t.logger).Log("msg", "error parsing syslog stream", "err", err)
	syslogParsingErrors.Inc()
}

func (t *SyslogTarget) handleMessage(connLabels labels.Labels, msg syslog.Message) {
	if msg.Message() == nil {
		return
	}

	lb := labels.NewBuilder(connLabels)
	for k, v := range t.config.Labels {
		lb.Set(string(k), string(v))
	}
	if v := msg.SeverityLevel(); v != nil {
		lb.Set("__syslog_message_severity", *v)
	}
	if v := msg.FacilityLevel(); v != nil {
		lb.Set("__syslog_message_facility", *v)
	}
	if v := msg.Hostname(); v != nil {
		lb.Set("__syslog_message_hostname", *v)
	}
	if v := msg.Appname(); v != nil {
		lb.Set("__syslog_message_app_name", *v)
	}
	if v := msg.ProcID(); v != nil {
		lb.Set("__syslog_message_proc_id", *v)
	}
	if v := msg.MsgID(); v != nil {
		lb.Set("__syslog_message_msg_id", *v)
	}

	if t.config.LabelStructuredData && msg.StructuredData() != nil {
		for id, params := range *msg.StructuredData() {
			id = strings.Replace(id, "@", "_", -1)
			for name, value := range params {
				key := "__syslog_message_sd_" + id + "_" + name
				lb.Set(key, value)
			}
		}
	}

	processed := relabel.Process(lb.Labels(), t.relabelConfig...)

	filtered := make(model.LabelSet)
	for _, lbl := range processed {
		if len(lbl.Name) >= 2 && lbl.Name[0:2] == "__" {
			continue
		}
		filtered[model.LabelName(lbl.Name)] = model.LabelValue(lbl.Value)
	}

	t.messages <- message{filtered, *msg.Message()}
}

func (t *SyslogTarget) messageSender() {
	for msg := range t.messages {
		t.handler.Handle(msg.labels, time.Now(), msg.message)
		syslogEntries.Inc()
	}
}

// Type returns SyslogTargetType.
func (t *SyslogTarget) Type() TargetType {
	return SyslogTargetType
}

// Ready indicates whether or not the syslog target is ready to be read from.
func (t *SyslogTarget) Ready() bool {
	return true
}

// DiscoveredLabels returns the set of labels discovered by the syslog target, which
// is always nil. Implements Target.
func (t *SyslogTarget) DiscoveredLabels() model.LabelSet {
	return nil
}

// Labels returns the set of labels that statically apply to all log entries
// produced by the SyslogTarget.
func (t *SyslogTarget) Labels() model.LabelSet {
	return t.config.Labels
}

// Details returns target-specific details.
func (t *SyslogTarget) Details() interface{} {
	return map[string]string{}
}

// Stop shuts down the SyslogTarget.
func (t *SyslogTarget) Stop() error {
	err := t.server.Stop()
	t.shutdown.Wait()
	close(t.messages)
	return err
}

// ListenAddress returns the address SyslogTarget is listening on.
func (t *SyslogTarget) ListenAddress() net.Addr {
	return t.server.Addr()
}
