package symbolicatorprocessor

import (
	"context"
	"fmt"
	"math"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/honeycombio/opentelemetry-collector-symbolicator/symbolicatorprocessor/internal/metadata"
	"github.com/honeycombio/symbolic-go"
)

type sourceMapStore interface {
	GetSourceMap(ctx context.Context, url string) ([]byte, []byte, error)
}

type basicSymbolicator struct {
	store   sourceMapStore
	timeout time.Duration
	ch      chan struct{}
	cache   *lru.Cache[string, *symbolic.SourceMapCache]
	telemetryBuilder *metadata.TelemetryBuilder
}

func newBasicSymbolicator(_ context.Context, timeout time.Duration, sourceMapCacheSize int, store sourceMapStore, tb *metadata.TelemetryBuilder) (*basicSymbolicator, error) {
	cache, err := lru.New[string, *symbolic.SourceMapCache](sourceMapCacheSize) // Adjust the size as needed

	if err != nil {
		return nil, err
	}
	return &basicSymbolicator{
		store:   store,
		timeout: timeout,
		// the channel is buffered to allow for a single request to be in progress at a time
		ch:    make(chan struct{}, 1),
		cache: cache,
		telemetryBuilder: tb,
	}, nil
}

type mappedStackFrame struct {
	FunctionName string
	URL          string
	Line         int64
	Col          int64
}

// symbolicate takes a line, column, function name, and URL and returns a string
func (ns *basicSymbolicator) symbolicate(ctx context.Context, line, column int64, function, url string) (*mappedStackFrame, error) {
	if column < 0 || column > math.MaxUint32 {
		return &mappedStackFrame{}, fmt.Errorf("column must be uint32: %d", column)
	}

	if line < 0 || line > math.MaxUint32 {
		return &mappedStackFrame{}, fmt.Errorf("line must be uint32: %d", line)
	}

	t, err := ns.limitedSymbolicate(ctx, line, column, url)

	if err != nil {
		return &mappedStackFrame{}, err
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
func (ns *basicSymbolicator) limitedSymbolicate(ctx context.Context, line, column int64, url string) (*symbolic.SourceMapCacheToken, error) {
	select {
	case ns.ch <- struct{}{}:
	case <-time.After(ns.timeout):
		return nil, fmt.Errorf("timeout")
	}

	defer func() {
		<-ns.ch
	}()

	smc, ok := ns.cache.Get(url)

	if !ok {
		source, sMap, err := ns.store.GetSourceMap(ctx, url)

		if err != nil {
			return nil, err
		}

		smc, err = symbolic.NewSourceMapCache(string(source), string(sMap))
		if err != nil {
			return nil, err
		}

		ns.cache.Add(url, smc)
	}

	return smc.Lookup(uint32(line), uint32(column), 0)
}
