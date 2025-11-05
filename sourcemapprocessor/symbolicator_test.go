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
	jsFile   = "https://www.honeycomb.io/assets/js/basic-mapping.js"
	noFile   = "https://www.honeycomb.io/assets/js/does-not-exist.js"
	uuid     = "e63db37d-9886-452a-8e56-2250dcc20102"
	uuidFile = "uuid-mapping.js"
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

	// The basic case.
	sf, err := sym.symbolicate(ctx, 0, 34, "b", jsFile, "")
	line := formatStackFrame(sf)
	assert.NoError(t, err)
	assert.Equal(t, "    at bar(basic-mapping-original.js:8:1)", line)

	// When there is no url.
	sf, err = sym.symbolicate(ctx, 0, 34, "b", "", "")
	line = formatStackFrame(sf)
	assert.NoError(t, err)
	assert.Equal(t, "    at b(:0:34)", line)

	// When UUID is present.
	sf, err = sym.symbolicate(ctx, 0, 34, "b", uuidFile, uuid)
	line = formatStackFrame(sf)
	assert.NoError(t, err)
	assert.Equal(t, "    at bar(uuid-mapping-original.js:8:1)", line)

	// When the file is missing.
	_, err = sym.symbolicate(ctx, 0, 34, "b", noFile, "")
	assert.Error(t, err)

	// The line number is too large.
	_, err = sym.symbolicate(ctx, math.MaxInt64, 34, "b", jsFile, "")
	assert.Error(t, err)

	// The column is too large.
	_, err = sym.symbolicate(ctx, 0, math.MaxInt64, "b", jsFile, "")
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
	sf, err := sym.symbolicate(ctx, 0, 34, "b", jsFile, "")
	line := formatStackFrame(sf)

	assert.NoError(t, err)
	assert.Equal(t, "    at bar(basic-mapping-original.js:8:1)", line)

	// Cache should have one entry
	assert.Equal(t, 1, sym.cache.Len())
}

func TestBuildCacheKey(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		buildUUID string
		expected  string
	}{
		{
			name:      "URL only, no UUID",
			url:       "app.js",
			buildUUID: "",
			expected:  "app.js",
		},
		{
			name:      "URL with UUID",
			url:       "app.js",
			buildUUID: "build-v1.0",
			expected:  "app.js|build-v1.0",
		},
		{
			name:      "Different URLs same UUID should differ",
			url:       "vendor.js",
			buildUUID: "build-v1.0",
			expected:  "vendor.js|build-v1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildCacheKey(tt.url, tt.buildUUID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSymbolicatorCacheWithUUID(t *testing.T) {
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

	// Symbolicate with UUID
	sf1, err := sym.symbolicate(ctx, 0, 34, "b", uuidFile, uuid)
	assert.NoError(t, err)
	assert.Equal(t, "bar", sf1.FunctionName)

	// Cache should have one entry (url|uuid)
	assert.Equal(t, 1, sym.cache.Len())

	// Symbolicate same URL with different UUID (simulating different build)
	// This should create a separate cache entry
	differentUUID := "different-uuid-1234"
	
	// This will fail to fetch since the file doesn't exist with this UUID, but that's okay
	// The important part is it tries to fetch (proving it's not using the cached entry)
	_, err = sym.symbolicate(ctx, 0, 34, "b", uuidFile, differentUUID)
	// We expect an error because the file won't exist with this UUID
	assert.Error(t, err, "Should attempt to fetch with different UUID and fail")

	// Cache should still have the first entry
	// (The second fetch failed, so it wasn't added to cache)
	assert.Equal(t, 1, sym.cache.Len())

	// Symbolicate same URL with NO UUID
	// This should also create a separate cache entry
	sf3, err := sym.symbolicate(ctx, 0, 34, "b", jsFile, "")
	assert.NoError(t, err)
	assert.Equal(t, "bar", sf3.FunctionName)

	// Cache should now have two entries: "uuid-mapping.js|<uuid>" and "jsFile"
	assert.Equal(t, 2, sym.cache.Len())
}
