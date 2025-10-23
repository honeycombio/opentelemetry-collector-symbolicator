package dsymprocessor

import (
	"context"
	"fmt"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/honeycombio/opentelemetry-collector-symbolicator/dsymprocessor/internal/metadata"
	"github.com/honeycombio/symbolic-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type dsymStore interface {
	GetDSYM(ctx context.Context, debugId, binaryName string) ([]byte, error)
}

type basicSymbolicator struct {
	store         dsymStore
	timeout       time.Duration
	ch            chan struct{}
	cache         *lru.Cache[string, *symbolic.Archive]
	notFoundCache *lru.Cache[string, struct{}] // cache for files that don't exist

	telemetryBuilder *metadata.TelemetryBuilder
	attributes       metric.MeasurementOption
}

func newBasicSymbolicator(_ context.Context, timeout time.Duration, cacheSize int, store dsymStore, tb *metadata.TelemetryBuilder, attributes attribute.Set) (*basicSymbolicator, error) {
	cache, err := lru.New[string, *symbolic.Archive](cacheSize)
	if err != nil {
		return nil, err
	}

	// Create a separate cache for "not found" entries to prevent repeated S3 calls
	// Use the same size as the main cache
	notFoundCache, err := lru.New[string, struct{}](cacheSize)
	if err != nil {
		return nil, err
	}

	return &basicSymbolicator{
		store:            store,
		timeout:          timeout,
		ch:               make(chan struct{}, 1), // buffered to allow for a single request to be in progress at a time
		cache:            cache,
		notFoundCache:    notFoundCache,
		telemetryBuilder: tb,
		attributes:       metric.WithAttributeSet(attributes),
	}, nil
}

type mappedDSYMStackFrame struct {
	path      string
	instrAddr uint64
	lang      string
	line      uint32
	symAddr   uint64
	symbol    string
}

func (ns *basicSymbolicator) symbolicateFrame(ctx context.Context, debugId, binaryName string, addr uint64) ([]*mappedDSYMStackFrame, error) {
	select {
	case ns.ch <- struct{}{}:
	case <-time.After(ns.timeout):
		return nil, fmt.Errorf("timeout")
	}

	defer func() {
		<-ns.ch
	}()

	cacheKey := debugId + "/" + binaryName

	// Check if we've previously determined this file doesn't exist (negative cache)
	if _, notFound := ns.notFoundCache.Get(cacheKey); notFound {
		// File is known to not exist, don't attempt to fetch again
		return nil, fmt.Errorf("dSYM not found (cached): %s", cacheKey)
	}

	archive, ok := ns.cache.Get(cacheKey)
	ns.telemetryBuilder.ProcessorDsymCacheSize.Record(ctx, int64(ns.cache.Len()), ns.attributes)

	if !ok {
		dSYMbytes, err := ns.store.GetDSYM(ctx, debugId, binaryName)
		if err != nil {
			ns.telemetryBuilder.ProcessorTotalDsymFetchFailures.Add(ctx, 1, ns.attributes)
			// Cache the fact that this file doesn't exist to prevent repeated S3 calls
			ns.notFoundCache.Add(cacheKey, struct{}{})
			return nil, err
		}
		archive, err = symbolic.NewArchiveFromBytes(dSYMbytes)

		if err != nil {
			// Don't cache parsing errors as they might be transient
			return nil, err
		}

		ns.cache.Add(cacheKey, archive)
	}

	// If the cache size has changed, we should record the new size
	ns.telemetryBuilder.ProcessorDsymCacheSize.Record(ctx, int64(ns.cache.Len()), ns.attributes)

	symCache, ok := archive.SymCaches[strings.ToLower(debugId)]
	if !ok {
		return nil, fmt.Errorf("could not find symcache for uuid %s", debugId)
	}

	locations, err := symCache.Lookup(addr)

	if err != nil {
		return nil, err
	}
	if len(locations) == 0 {
		return nil, fmt.Errorf("could not find symbol at location %d", addr)
	}

	res := make([]*mappedDSYMStackFrame, len(locations))
	for i, loc := range locations {
		res[i] = &mappedDSYMStackFrame{
			path:      loc.FullPath,
			instrAddr: loc.InstrAddr,
			lang:      loc.Lang,
			line:      loc.Line,
			symAddr:   loc.SymAddr,
			symbol:    loc.Symbol,
		}
	}
	return res, nil
}
