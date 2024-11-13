package symbolicatorprocessor

import (
	"testing"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap/zaptest"
)

func TestNewSymbolicatorProcessor(t *testing.T) {
	cfg := &Config{}
	logger := zaptest.NewLogger(t)
	set := processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: logger,
		},
	}

	sp := newSymbolicatorProcessor(cfg, set)

	if sp == nil {
		t.Error("Expected non-nil symbolicatorProcessor")
	}

	if sp.cfg != cfg {
		t.Errorf("Expected cfg to be %v, got %v", cfg, sp.cfg)
	}

	if sp.logger != logger {
		t.Errorf("Expected logger to be %v, got %v", logger, sp.logger)
	}
}
