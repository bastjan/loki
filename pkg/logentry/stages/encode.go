package stages

import (
	"fmt"
	"reflect"
	"time"
	"unicode/utf8"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
)

// Config Errors
const (
	ErrEmptyEncodingStageConfig = "empty encoding stage configuration"
	ErrEncodingRequired         = "encoding is required"
	ErrEmptyEncodingStageSource = "empty source in encoding stage"
)

// EncodingConfig contains a regexStage configuration
type EncodingConfig struct {
	Encoding string  `mapstructure:"encoding"`
	Source   *string `mapstructure:"source"`
}

// validateEncodingConfig validates the config and return a Decoder
func validateEncodingConfig(c *EncodingConfig) (*encoding.Decoder, error) {
	if c == nil {
		return nil, errors.New(ErrEmptyEncodingStageConfig)
	}

	if c.Source != nil && *c.Source == "" {
		return nil, errors.New(ErrEmptyEncodingStageSource)
	}

	if c.Encoding == "" {
		return nil, errors.New(ErrEncodingRequired)
	}

	enc, err := findEncoding(c.Encoding)
	if err != nil {
		return nil, err
	}
	return enc.NewDecoder(), nil
}

// encodingStage sets extracted data using regular expressions
type encodingStage struct {
	cfg     *EncodingConfig
	decoder *encoding.Decoder
	logger  log.Logger
}

// newEncodingStage creates a newEncodingStage
func newEncodingStage(logger log.Logger, config interface{}) (Stage, error) {
	cfg, err := parseEncodingConfig(config)
	if err != nil {
		return nil, err
	}
	decoder, err := validateEncodingConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &encodingStage{
		cfg:     cfg,
		decoder: decoder,
		logger:  log.With(logger, "component", "stage", "type", "encoding"),
	}, nil
}

// parseEncodingConfig processes an incoming configuration into a EncodingConfig
func parseEncodingConfig(config interface{}) (*EncodingConfig, error) {
	cfg := &EncodingConfig{}
	err := mapstructure.Decode(config, cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func findEncoding(enc string) (encoding.Encoding, error) {
	for _, cm := range charmap.All {
		if enc == cm.(fmt.Stringer).String() {
			return cm, nil
		}
	}

	return nil, fmt.Errorf("encoding '%s' not found", enc)
}

// Process implements Stage
func (r *encodingStage) Process(labels model.LabelSet, extracted map[string]interface{}, t *time.Time, entry *string) {
	// If a source key is provided, the encoding stage should process it
	// from the extracted map, otherwise should fallback to the entry
	input := entry

	if r.cfg.Source != nil {
		if _, ok := extracted[*r.cfg.Source]; !ok {
			if Debug {
				level.Debug(r.logger).Log("msg", "source does not exist in the set of extracted values", "source", *r.cfg.Source)
			}
			return
		}

		value, err := getString(extracted[*r.cfg.Source])
		if err != nil {
			if Debug {
				level.Debug(r.logger).Log("msg", "failed to convert source value to string", "source", *r.cfg.Source, "err", err, "type", reflect.TypeOf(extracted[*r.cfg.Source]))
			}
			return
		}

		input = &value
	}

	if input == nil {
		if Debug {
			level.Debug(r.logger).Log("msg", "cannot decode a nil entry")
		}
		return
	}

	if utf8.ValidString(*input) {
		if Debug {
			level.Debug(r.logger).Log("msg", "input is already valid utf8", "input", *input, "encoding", r.cfg.Encoding)
		}
		return
	}

	result, err := r.decoder.String(*input)
	if err != nil {
		if Debug {
			level.Debug(r.logger).Log("msg", "decoder returned error", "input", *input, "encoding", r.cfg.Encoding)
		}
		return
	}

	if r.cfg.Source != nil {
		extracted[*r.cfg.Source] = result
		return
	}

	*entry = result
}

// Name implements Stage
func (r *encodingStage) Name() string {
	return StageTypeEncoding
}
