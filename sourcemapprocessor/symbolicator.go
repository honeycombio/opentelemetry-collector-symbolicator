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

type sourceMapStore interface {
	GetSourceMap(ctx context.Context, url string, uuid string) ([]byte, []byte, error)
}

type basicSymbolicator struct {
	store         sourceMapStore
	timeout       time.Duration
	ch            chan struct{}
	cache         *lru.Cache[string, *symbolic.SourceMapCache]
	notFoundCache *lru.Cache[string, struct{}] // cache for files that don't exist

	telemetryBuilder *metadata.TelemetryBuilder
	attributes       metric.MeasurementOption
}

func newBasicSymbolicator(_ context.Context, timeout time.Duration, sourceMapCacheSize int, store sourceMapStore, tb *metadata.TelemetryBuilder, attributes attribute.Set) (*basicSymbolicator, error) {
	cache, err := lru.New[string, *symbolic.SourceMapCache](sourceMapCacheSize)
	if err != nil {
		return nil, err
	}

	// Create a separate cache for "not found" entries to prevent repeated S3 calls
	// Use the same size as the main cache
	notFoundCache, err := lru.New[string, struct{}](sourceMapCacheSize)
	if err != nil {
		return nil, err
	}

	return &basicSymbolicator{
		store:         store,
		timeout:       timeout,
		ch:            make(chan struct{}, 1), // buffered to allow for a single request to be in progress at a time
		cache:         cache,
		notFoundCache: notFoundCache,
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

	// Check if we've previously determined this file doesn't exist (negative cache)
	if _, notFound := ns.notFoundCache.Get(url); notFound {
		// File is known to not exist, don't attempt to fetch again
		return nil, fmt.Errorf("source map not found (cached): %s", url)
	}

	smc, ok := ns.cache.Get(url)
	ns.telemetryBuilder.ProcessorSourceMapCacheSize.Record(ctx, int64(ns.cache.Len()), ns.attributes)

	if !ok {
		source, sMap, err := ns.store.GetSourceMap(ctx, url, uuid)

		if err != nil {
			ns.telemetryBuilder.ProcessorTotalSourceMapFetchFailures.Add(ctx, 1, ns.attributes)
			// Cache the fact that this file doesn't exist to prevent repeated S3 calls
			ns.notFoundCache.Add(url, struct{}{})
			return nil, err
		}

		smc, err = symbolic.NewSourceMapCache(string(source), string(sMap))
		if err != nil {
			// Don't cache parsing errors as they might be transient
			return nil, err
		}

		ns.cache.Add(url, smc)
	}

	// If the cache size has changed, we should record the new size
	ns.telemetryBuilder.ProcessorSourceMapCacheSize.Record(ctx, int64(ns.cache.Len()), ns.attributes)
	return smc.Lookup(uint32(line), uint32(column), 0)
}
