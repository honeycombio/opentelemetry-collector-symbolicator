package sourcemapprocessor

import (
	"context"
	"fmt"
	"math"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/honeycombio/opentelemetry-collector-symbolicator/sourcemapprocessor/internal/metadata"
	"github.com/honeycombio/symbolic-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// buildCacheKey creates a consistent cache key for source maps and error caching.
// Includes buildUUID when present to ensure different builds are cached separately.
func buildCacheKey(url, buildUUID string) string {
	if buildUUID == "" {
		return url
	}
	return url + "|" + buildUUID
}

type sourceMapStore interface {
	GetSourceMap(ctx context.Context, url string, uuid string) ([]byte, []byte, error)
}

type basicSymbolicator struct {
	store   sourceMapStore
	timeout time.Duration
	ch      chan struct{}
	cache   *lru.Cache[string, *symbolic.SourceMapCache]

	telemetryBuilder *metadata.TelemetryBuilder
	attributes       metric.MeasurementOption
}

func newBasicSymbolicator(_ context.Context, timeout time.Duration, sourceMapCacheSize int, store sourceMapStore, tb *metadata.TelemetryBuilder, attributes attribute.Set) (*basicSymbolicator, error) {
	cache, err := lru.New[string, *symbolic.SourceMapCache](sourceMapCacheSize) // Adjust the size as needed

	if err != nil {
		return nil, err
	}
	return &basicSymbolicator{
		store:   store,
		timeout: timeout,
		// the channel is buffered to allow for a single request to be in progress at a time
		ch:               make(chan struct{}, 1),
		cache:            cache,
		telemetryBuilder: tb,
		attributes:       metric.WithAttributeSet(attributes),
	}, nil
}

type mappedStackFrame struct {
	FunctionName string
	URL          string
	Line         int64
	Col          int64
}

// symbolicate takes a line, column, function name, and URL and returns a string
func (ns *basicSymbolicator) symbolicate(ctx context.Context, line, column int64, function, url, uuid string) (*mappedStackFrame, error) {
	if column < 0 || column > math.MaxUint32 {
		return nil, fmt.Errorf("column must be uint32: %d", column)
	}

	if line < 0 || line > math.MaxUint32 {
		return nil, fmt.Errorf("line must be uint32: %d", line)
	}

	if url == "" {
		// If there is no URL, then it's something like "native", so pass it through directly.
		return &mappedStackFrame{
			FunctionName: function,
			URL:          url,
			Line:         int64(line),
			Col:          int64(column),
		}, nil
	}

	t, err := ns.limitedSymbolicate(ctx, line, column, url, uuid)

	if err != nil {
		return nil, err
	}

	return &mappedStackFrame{
		FunctionName: t.FunctionName,
		URL:          t.Src,
		Line:         int64(t.Line),
		Col:          int64(t.Col),
	}, nil
}

// limitedSymbolicate performs the actual symbolication. It is limited to a single request at a time
// it checks and caches the source map cache before loading the source map from the store
func (ns *basicSymbolicator) limitedSymbolicate(ctx context.Context, line, column int64, url, uuid string) (*symbolic.SourceMapCacheToken, error) {
	select {
	case ns.ch <- struct{}{}:
	case <-time.After(ns.timeout):
		return nil, fmt.Errorf("timeout")
	}

	defer func() {
		<-ns.ch
	}()

	cacheKey := buildCacheKey(url, uuid)
	smc, ok := ns.cache.Get(cacheKey)
	ns.telemetryBuilder.ProcessorSourceMapCacheSize.Record(ctx, int64(ns.cache.Len()), ns.attributes)

	if !ok {
		source, sMap, err := ns.store.GetSourceMap(ctx, url, uuid)

		if err != nil {
			ns.telemetryBuilder.ProcessorTotalSourceMapFetchFailures.Add(ctx, 1, ns.attributes)
			return nil, err
		}

		smc, err = symbolic.NewSourceMapCache(string(source), string(sMap))
		if err != nil {
			return nil, err
		}

		ns.cache.Add(cacheKey, smc)
	}

	// If the cache size has changed, we should record the new size
	ns.telemetryBuilder.ProcessorSourceMapCacheSize.Record(ctx, int64(ns.cache.Len()), ns.attributes)
	return smc.Lookup(uint32(line), uint32(column), 0)
}
