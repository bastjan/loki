package targets

import (
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/relabel"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/grafana/loki/pkg/promtail/scrape"
)

type ClientMessage struct {
	Labels    model.LabelSet
	Timestamp time.Time
	Message   string
}

type TestLabeledClient struct {
	log      log.Logger
	messages []ClientMessage
	sync.RWMutex
}

func (c *TestLabeledClient) Handle(ls model.LabelSet, t time.Time, s string) error {
	level.Debug(c.log).Log("msg", "received log", "log", s)

	c.Lock()
	defer c.Unlock()
	c.messages = append(c.messages, ClientMessage{ls, t, s})
	return nil
}

func (c *TestLabeledClient) Messages() []ClientMessage {
	c.RLock()
	defer c.RUnlock()

	return c.messages
}

func TestSyslogTarget_NewlineSeparatedMessages(t *testing.T) {
	syslogTest{
		network: "tcp",
		formatter: func(s string) string {
			return s + "\n"
		},
	}.run(t)
}

func TestSyslogTarget_OctetCounting(t *testing.T) {
	syslogTest{
		network: "tcp",
		formatter: func(s string) string {
			return fmt.Sprintf("%d %s", len(s), s)
		},
	}.run(t)
}

func TestSyslogTarget_UDP(t *testing.T) {
	syslogTest{
		network:   "udp",
		formatter: func(s string) string { return s },
	}.run(t)
}

type syslogTest struct {
	network   string
	needsSort bool
	formatter func(string) string
}

func (s syslogTest) run(t *testing.T) {
	w := log.NewSyncWriter(os.Stderr)
	logger := log.NewLogfmtLogger(w)
	client := &TestLabeledClient{log: logger}

	tgt, err := NewSyslogTarget(logger, client, relabelConfig(t), &scrape.SyslogTargetConfig{
		ListenAddress:       "127.0.0.1:0/" + s.network,
		LabelStructuredData: true,
		Labels: model.LabelSet{
			"test": "syslog_target",
		},
	})
	require.NoError(t, err)
	defer tgt.Stop()

	addr := tgt.ListenAddress().String()
	c, err := net.Dial(s.network, addr)
	require.NoError(t, err)
	defer c.Close()

	messages := []string{
		`<165>1 2018-10-11T22:14:15.003Z host5 e - id1 [custom@32473 exkey="1"] An application event log entry...`,
		`<165>1 2018-10-11T22:14:15.005Z host5 e - id2 [custom@32473 exkey="2"] An application event log entry...`,
		`<165>1 2018-10-11T22:14:15.007Z host5 e - id3 [custom@32473 exkey="3"] An application event log entry...`,
	}

	err = writeMessagesToStream(c, messages, s.formatter)
	require.NoError(t, err)

	require.Eventuallyf(t, func() bool {
		return len(client.Messages()) == len(messages)
	}, time.Second, time.Millisecond, "Expected to receive %d messages, got %d.", len(messages), len(client.Messages()))

	received := client.Messages()

	if s.needsSort {
		sort.Slice(received, func(i, j int) bool {
			ii, _ := strconv.Atoi(string(received[i].Labels["sd_custom_exkey"]))
			ji, _ := strconv.Atoi(string(received[j].Labels["sd_custom_exkey"]))
			return ii < ji
		})
	}

	require.Equal(t, model.LabelSet{
		"test": "syslog_target",

		"severity": "notice",
		"facility": "local4",
		"hostname": "host5",
		"app_name": "e",
		"msg_id":   "id1",

		"sd_custom_exkey": "1",
	}, received[0].Labels)
	require.Equal(t, "An application event log entry...", received[0].Message)

	require.NotZero(t, received[0].Timestamp)
}

func relabelConfig(t *testing.T) []*relabel.Config {
	relabelCfg := `
- source_labels: ['__syslog_message_severity']
  target_label: 'severity'
- source_labels: ['__syslog_message_facility']
  target_label: 'facility'
- source_labels: ['__syslog_message_hostname']
  target_label: 'hostname'
- source_labels: ['__syslog_message_app_name']
  target_label: 'app_name'
- source_labels: ['__syslog_message_proc_id']
  target_label: 'proc_id'
- source_labels: ['__syslog_message_msg_id']
  target_label: 'msg_id'
- source_labels: ['__syslog_message_sd_custom_32473_exkey']
  target_label: 'sd_custom_exkey'
`

	var relabels []*relabel.Config
	err := yaml.Unmarshal([]byte(relabelCfg), &relabels)
	require.NoError(t, err)

	return relabels
}

func writeMessagesToStream(w io.Writer, messages []string, formatter func(string) string) error {
	for _, msg := range messages {
		_, err := fmt.Fprint(w, formatter(msg))
		if err != nil {
			return err
		}
	}

	return nil
}
