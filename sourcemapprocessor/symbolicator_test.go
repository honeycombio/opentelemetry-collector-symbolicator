package sourcemapprocessor

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/sourcemapprocessor/internal/metadata"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap/zaptest"
)

var (
	jsFile = "https://www.honeycomb.io/assets/js/basic-mapping.js"
	noFile = "https://www.honeycomb.io/assets/js/does-not-exist.js"
)

func TestSymbolicator(t *testing.T) {
	ctx := context.Background()

	testTel := componenttest.NewTelemetry()
	tb, err := metadata.NewTelemetryBuilder(testTel.NewTelemetrySettings())
	assert.NoError(t, err)
	defer tb.Shutdown()

	attributes := attribute.NewSet(
		attribute.String("processor_type", "symbolicator"),
	)

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalSourceMapConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)
	sym, _ := newBasicSymbolicator(ctx, 5*time.Second, 128, fs, tb, attributes)

	sf, err := sym.symbolicate(ctx, 0, 34, "b", jsFile)
	line := formatStackFrame(sf)

	assert.NoError(t, err)
	assert.Equal(t, "    at bar(basic-mapping-original.js:8:1)", line)

	_, err = sym.symbolicate(ctx, 0, 34, "b", noFile)
	assert.Error(t, err)

	_, err = sym.symbolicate(ctx, math.MaxInt64, 34, "b", jsFile)
	assert.Error(t, err)

	_, err = sym.symbolicate(ctx, 0, math.MaxInt64, "b", jsFile)
	assert.Error(t, err)
}

func TestSymbolicatorCache(t *testing.T) {
	ctx := context.Background()

	testTel := componenttest.NewTelemetry()
	tb, err := metadata.NewTelemetryBuilder(testTel.NewTelemetrySettings())
	assert.NoError(t, err)
	defer tb.Shutdown()

	attributes := attribute.NewSet(
		attribute.String("processor_type", "symbolicator"),
	)

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalSourceMapConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)
	sym, _ := newBasicSymbolicator(ctx, 5*time.Second, 128, fs, tb, attributes)

	// Cache should be empty to start
	assert.Equal(t, 0, sym.cache.Len())

	// First symbolication should add to cache
	sf, err := sym.symbolicate(ctx, 0, 34, "b", jsFile)
	line := formatStackFrame(sf)

	assert.NoError(t, err)
	assert.Equal(t, "    at bar(basic-mapping-original.js:8:1)", line)

	// Cache should have one entry
	assert.Equal(t, 1, sym.cache.Len())
}
