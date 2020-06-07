package stages

import (
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/cortexproject/cortex/pkg/util"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/charmap"
	"gopkg.in/yaml.v2"
)

var testEncodingYamlSingleStageWithoutSource = `
pipeline_stages:
- encoding:
    expression: "11.11.11.11 - (\\S+) .*"
    encoding: "dummy"
`
var testEncodingYamlMultiStageWithSource = `
pipeline_stages:
- json:
    expressions:
      level:
      msg:
- encoding:
    expression: "\\S+ - \"POST (\\S+) .*"
    source: msg
    encoding: "/loki/api/v1/push/"
`

var testEncodingYamlWithNamedCaputedGroupWithTemplate = `
---
pipeline_stages:
  -
    encoding:
      expression: "^(?P<ip>\\S+) (?P<identd>\\S+) (?P<user>\\S+) \\[(?P<timestamp>[\\w:/]+\\s[+\\-]\\d{4})\\] \"(?P<action>\\S+)\\s?(?P<path>\\S+)?\\s?(?P<protocol>\\S+)?\" (?P<status>\\d{3}|-) (\\d+|-)\\s?\"?(?P<referer>[^\"]*)\"?\\s?\"?(?P<useragent>[^\"]*)?\"?$"
      encoding: '{{ if eq .Value "200" }}{{ Encoding .Value "200" "HttpStatusOk" -1 }}{{ else }}{{ .Value | ToUpper }}{{ end }}'
`

var testEncodingYamlWithTemplate = `
---
pipeline_stages:
  -
    encoding:
      expression: "^(\\S+) (\\S+) (\\S+) \\[([\\w:/]+\\s[+\\-]\\d{4})\\] \"(\\S+)\\s?(\\S+)?\\s?(\\S+)?\" (\\d{3}|-) (\\d+|-)\\s?\"?([^\"]*)\"?\\s?\"?([^\"]*)?\"?$"
      encoding: '{{ if eq .Value "200" }}{{ Encoding .Value "200" "HttpStatusOk" -1 }}{{ else }}{{ .Value | ToUpper }}{{ end }}'
`

var testEncodingLogLine = `11.11.11.11 - frank [25/Jan/2000:14:00:01 -0500] "GET /1986.js HTTP/1.1" 200 932 "-" "Mozilla/5.0 (Windows; U; Windows NT 5.1; de; rv:1.9.1.7) Gecko/20091221 Firefox/3.5.7 GTB6"`
var testEncodingLogJSONLine = `{"time":"2019-01-01T01:00:00.000000001Z", "level": "info", "msg": "11.11.11.11 - \"POST /loki/api/push/ HTTP/1.1\" 200 932 \"-\" \"Mozilla/5.0 (Windows; U; Windows NT 5.1; de; rv:1.9.1.7) Gecko/20091221 Firefox/3.5.7 GTB6\""}`

func TestPipeline_Encoding(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		config        string
		entry         string
		extracted     map[string]interface{}
		expectedEntry string
	}{
		"successfully run a pipeline with 1 regex stage without source": {
			testEncodingYamlSingleStageWithoutSource,
			testEncodingLogLine,
			map[string]interface{}{},
			`11.11.11.11 - dummy [25/Jan/2000:14:00:01 -0500] "GET /1986.js HTTP/1.1" 200 932 "-" "Mozilla/5.0 (Windows; U; Windows NT 5.1; de; rv:1.9.1.7) Gecko/20091221 Firefox/3.5.7 GTB6"`,
		},
		"successfully run a pipeline with multi stage with": {
			testEncodingYamlMultiStageWithSource,
			testEncodingLogJSONLine,
			map[string]interface{}{
				"level": "info",
				"msg":   `11.11.11.11 - "POST /loki/api/v1/push/ HTTP/1.1" 200 932 "-" "Mozilla/5.0 (Windows; U; Windows NT 5.1; de; rv:1.9.1.7) Gecko/20091221 Firefox/3.5.7 GTB6"`,
			},
			`{"time":"2019-01-01T01:00:00.000000001Z", "level": "info", "msg": "11.11.11.11 - \"POST /loki/api/push/ HTTP/1.1\" 200 932 \"-\" \"Mozilla/5.0 (Windows; U; Windows NT 5.1; de; rv:1.9.1.7) Gecko/20091221 Firefox/3.5.7 GTB6\""}`,
		},
		"successfully run a pipeline with 1 regex stage with named captured group and with template and without source": {
			testEncodingYamlWithNamedCaputedGroupWithTemplate,
			testEncodingLogLine,
			map[string]interface{}{
				"ip":        "11.11.11.11",
				"identd":    "-",
				"user":      "FRANK",
				"timestamp": "25/JAN/2000:14:00:01 -0500",
				"action":    "GET",
				"path":      "/1986.JS",
				"protocol":  "HTTP/1.1",
				"status":    "HttpStatusOk",
				"referer":   "-",
				"useragent": "MOZILLA/5.0 (WINDOWS; U; WINDOWS NT 5.1; DE; RV:1.9.1.7) GECKO/20091221 FIREFOX/3.5.7 GTB6",
			},
			`11.11.11.11 - FRANK [25/JAN/2000:14:00:01 -0500] "GET /1986.JS HTTP/1.1" HttpStatusOk 932 "-" "MOZILLA/5.0 (WINDOWS; U; WINDOWS NT 5.1; DE; RV:1.9.1.7) GECKO/20091221 FIREFOX/3.5.7 GTB6"`,
		},
		"successfully run a pipeline with 1 regex stage with template and without source": {
			testEncodingYamlWithTemplate,
			testEncodingLogLine,
			map[string]interface{}{},
			`11.11.11.11 - FRANK [25/JAN/2000:14:00:01 -0500] "GET /1986.JS HTTP/1.1" HttpStatusOk 932 "-" "MOZILLA/5.0 (WINDOWS; U; WINDOWS NT 5.1; DE; RV:1.9.1.7) GECKO/20091221 FIREFOX/3.5.7 GTB6"`,
		},
	}

	for testName, testData := range tests {
		testData := testData

		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			pl, err := NewPipeline(util.Logger, loadConfig(testData.config), nil, prometheus.DefaultRegisterer)
			if err != nil {
				t.Fatal(err)
			}

			lbls := model.LabelSet{}
			ts := time.Now()
			entry := testData.entry
			extracted := map[string]interface{}{}
			pl.Process(lbls, extracted, &ts, &entry)
			assert.Equal(t, testData.expectedEntry, entry)
			assert.Equal(t, testData.extracted, extracted)
		})
	}
}

var encodingCfg = `
encoding:
  source: "source"
  encoding: "encoding"`

func TestEncodingMapStructure(t *testing.T) {
	t.Parallel()

	var mapstruct map[interface{}]interface{}
	if err := yaml.Unmarshal([]byte(encodingCfg), &mapstruct); err != nil {
		t.Fatalf("error while un-marshalling config: %s", err)
	}
	p, ok := mapstruct["encoding"].(map[interface{}]interface{})
	if !ok {
		t.Fatalf("could not read parser %+v", mapstruct["encoding"])
	}
	got, err := parseEncodingConfig(p)
	if err != nil {
		t.Fatalf("could not create parser from yaml: %s", err)
	}

	s := "source"
	want := &EncodingConfig{
		Source:   &s,
		Encoding: "encoding",
	}
	require.Equal(t, want, got)
}

func TestEncodingConfig_validate(t *testing.T) {
	tests := map[string]struct {
		config interface{}
		err    error
	}{
		"empty config": {
			nil,
			errors.New(ErrEncodingRequired),
		},
		"missing encoding": {
			map[string]interface{}{},
			errors.New(ErrEncodingRequired),
		},
		"invalid encoding": {
			map[string]interface{}{
				"encoding": "test",
			},
			errors.New("encoding 'test' not found"),
		},
		"empty source": {
			map[string]interface{}{
				"source":   "",
				"encoding": "ISO 8859-13",
			},
			errors.New(ErrEmptyEncodingStageSource),
		},
		"empty encoding": {
			map[string]interface{}{
				"encoding": "",
			},
			errors.New(ErrEncodingRequired),
		},
		"valid without source": {
			map[string]interface{}{
				"encoding": "ISO 8859-13",
			},
			nil,
		},
		"valid with source": {
			map[string]interface{}{
				"source":   "log",
				"encoding": "ISO 8859-13",
			},
			nil,
		},
	}
	for tName, tt := range tests {
		tt := tt
		t.Run(tName, func(t *testing.T) {
			t.Parallel()

			c, err := parseEncodingConfig(tt.config)
			if err != nil {
				t.Fatalf("failed to create config: %s", err)
			}
			_, err = validateEncodingConfig(c)
			if (err != nil) != (tt.err != nil) {
				t.Errorf("EncodingConfig.validate() expected error = %v, actual error = %v", tt.err, err)
				return
			}
			if (err != nil) && (err.Error() != tt.err.Error()) {
				t.Errorf("EncodingConfig.validate() expected error = %v, actual error = %v", tt.err, err)
				return
			}
		})
	}
}

func TestEncoding_DecodeISOLatin1(t *testing.T) {
	w := log.NewSyncWriter(ioutil.Discard)
	logger := log.NewLogfmtLogger(w)

	conf := map[interface{}]interface{}{
		"encoding": "ISO 8859-1",
	}

	stage, err := newEncodingStage(logger, conf)
	require.NoError(t, err)

	subject := "chääs"
	subjectEncoded, err := latin1String(subject)
	require.NoError(t, err)

	tm := time.Now()
	stage.Process(nil, nil, &tm, &subjectEncoded)
	assert.Equal(t, subject, subjectEncoded)
}

func TestEncoding_DecodeISOLatin1Source(t *testing.T) {
	w := log.NewSyncWriter(ioutil.Discard)
	logger := log.NewLogfmtLogger(w)

	conf := map[interface{}]interface{}{
		"encoding": "ISO 8859-1",
		"source":   "somefield",
	}

	stage, err := newEncodingStage(logger, conf)
	require.NoError(t, err)

	subject := "chääs"
	subjectEncoded, err := latin1String(subject)
	require.NoError(t, err)

	extracted := map[string]interface{}{
		"somefield": subjectEncoded,
	}

	tm := time.Now()
	stage.Process(nil, extracted, &tm, nil)
	assert.Equal(t, subject, extracted["somefield"])
}

func latin1String(s string) (string, error) {
	return charmap.ISO8859_1.NewEncoder().String(s)
}

func TestEncoding_PrintSupported(t *testing.T) {
	for _, enc := range charmap.All {
		fmt.Println("-", enc)
	}
}
