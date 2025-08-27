package proguardprocessor

import (
	"context"
	"fmt"
	"os"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/honeycombio/opentelemetry-collector-symbolicator/proguardprocessor/internal/metadata"
	"github.com/honeycombio/symbolic-go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type fileStore interface {
	GetProguardMapping(ctx context.Context, uuid string) ([]byte, error)
}

type basicSymbolicator struct {
	store   fileStore
	timeout time.Duration
	ch      chan struct{}
	cache   *lru.Cache[string, *symbolic.ProguardMapper]

	telemetryBuilder *metadata.TelemetryBuilder
	attributes       metric.MeasurementOption
}

func newBasicSymbolicator(_ context.Context, timeout time.Duration, cacheSize int, store fileStore, tb *metadata.TelemetryBuilder, attributes attribute.Set) (*basicSymbolicator, error) {
	cache, err := lru.New[string, *symbolic.ProguardMapper](cacheSize) // Adjust the size as needed

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
	ClassName      string
	MethodName     string
	LineNumber     int64
	SourceFile     string
	ParameterNames string
}

// symbolicate takes a line, column, function name, and URL and returns a string
func (ns *basicSymbolicator) symbolicate(ctx context.Context, uuid, class, method string, line int) ([]*mappedStackFrame, error) {
	t, err := ns.limitedSymbolicate(ctx, uuid, class, method, line)

	if err != nil {
		return nil, err
	}

	msfs := make([]*mappedStackFrame, 0, len(t))

	for _, sf := range t {
		msf := &mappedStackFrame{
			ClassName:      sf.ClassName,
			MethodName:     sf.MethodName,
			LineNumber:     int64(sf.LineNumber),
			SourceFile:     sf.SourceFile,
			ParameterNames: sf.ParameterNames,
		}
		msfs = append(msfs, msf)
	}

	return msfs, nil
}

// limitedSymbolicate performs the actual symbolication. It is limited to a single request at a time
// it checks and caches the proguard cache before loading the proguard file from the store
func (ns *basicSymbolicator) limitedSymbolicate(ctx context.Context, uuid, class, method string, line int) ([]*symbolic.SymbolicJavaStackFrame, error) {
	select {
	case ns.ch <- struct{}{}:
	case <-time.After(ns.timeout):
		return nil, fmt.Errorf("timeout")
	}

	defer func() {
		<-ns.ch
	}()

	pm, ok := ns.cache.Get(uuid)
	ns.telemetryBuilder.ProcessorProguardCacheSize.Record(ctx, int64(ns.cache.Len()), ns.attributes)

	if !ok {
		pmf, err := ns.store.GetProguardMapping(ctx, uuid)

		if err != nil {
			ns.telemetryBuilder.ProcessorTotalProguardFetchFailures.Add(ctx, 1, ns.attributes)
			return nil, err
		}

		f, err := os.CreateTemp("", "proguard-*.txt")

		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}

		defer f.Close()
		defer os.Remove(f.Name())

		_, err = f.Write(pmf)

		if err != nil {
			return nil, fmt.Errorf("failed to write proguard mapping to temp file: %w", err)
		}

		pm, err = symbolic.NewProguardMapper(f.Name())
		if err != nil {
			return nil, err
		}

		ns.cache.Add(uuid, pm)
	}

	// If the cache size has changed, we should record the new size
	ns.telemetryBuilder.ProcessorProguardCacheSize.Record(ctx, int64(ns.cache.Len()), ns.attributes)
	return pm.RemapFrame(class, method, line)
}
