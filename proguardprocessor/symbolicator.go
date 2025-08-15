package proguardprocessor

import (
	"context"
	"fmt"
	"os"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/honeycombio/symbolic-go"
	"go.uber.org/zap"
)

type fileStore interface {
	GetProguardMapping(ctx context.Context, uuid string) ([]byte, error)
}

type basicSymbolicator struct {
	store   fileStore
	timeout time.Duration
	ch      chan struct{}
	cache   *lru.Cache[string, *symbolic.ProguardMapper]
	logger  *zap.Logger
}

func newBasicSymbolicator(_ context.Context, timeout time.Duration, cacheSize int, store fileStore, logger *zap.Logger) (*basicSymbolicator, error) {
	logger.Info("🔥 SYMBOLICATOR: Creating new basicSymbolicator", 
		zap.Duration("timeout", timeout),
		zap.Int("cache_size", cacheSize))

	cache, err := lru.New[string, *symbolic.ProguardMapper](cacheSize) // Adjust the size as needed

	if err != nil {
		logger.Error("🔥 SYMBOLICATOR: Failed to create LRU cache", zap.Error(err))
		return nil, err
	}

	logger.Info("🔥 SYMBOLICATOR: Successfully created basicSymbolicator")
	return &basicSymbolicator{
		store:   store,
		timeout: timeout,
		// the channel is buffered to allow for a single request to be in progress at a time
		ch:     make(chan struct{}, 1),
		cache:  cache,
		logger: logger,
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
	ns.logger.Info("🔥 SYMBOLICATOR: symbolicate called", 
		zap.String("uuid", uuid),
		zap.String("class", class),
		zap.String("method", method),
		zap.Int("line", line))

	t, err := ns.limitedSymbolicate(ctx, uuid, class, method, line)

	if err != nil {
		ns.logger.Error("🔥 SYMBOLICATOR: limitedSymbolicate failed", 
			zap.Error(err),
			zap.String("uuid", uuid),
			zap.String("class", class),
			zap.String("method", method),
			zap.Int("line", line))
		return nil, err
	}

	ns.logger.Info("🔥 SYMBOLICATOR: limitedSymbolicate succeeded", 
		zap.Int("result_frames", len(t)))

	msfs := make([]*mappedStackFrame, 0, len(t))

	for i, sf := range t {
		ns.logger.Debug("🔥 SYMBOLICATOR: Processing symbolic frame", 
			zap.Int("index", i),
			zap.String("className", sf.ClassName),
			zap.String("methodName", sf.MethodName),
			zap.Int("lineNumber", sf.LineNumber),
			zap.String("sourceFile", sf.SourceFile))

		msf := &mappedStackFrame{
			ClassName:      sf.ClassName,
			MethodName:     sf.MethodName,
			LineNumber:     int64(sf.LineNumber),
			SourceFile:     sf.SourceFile,
			ParameterNames: sf.ParameterNames,
		}
		ns.logger.Debug("🔥 SYMBOLICATOR: Processed symbolic frame", 
			zap.Int("index", i),
			zap.String("className", msf.ClassName),
			zap.String("methodName", msf.MethodName),
			zap.Int64("lineNumber", msf.LineNumber),
			zap.String("sourceFile", msf.SourceFile))
		msfs = append(msfs, msf) 
	}

	ns.logger.Info("🔥 SYMBOLICATOR: symbolicate completed successfully", 
		zap.Int("mapped_frames", len(msfs)))

	return msfs, nil
}

// limitedSymbolicate performs the actual symbolication. It is limited to a single request at a time
// it checks and caches the proguard cache before loading the proguard file from the store
func (ns *basicSymbolicator) limitedSymbolicate(ctx context.Context, uuid, class, method string, line int) ([]*symbolic.SymbolicJavaStackFrame, error) {
	ns.logger.Debug("🔥 SYMBOLICATOR: limitedSymbolicate called", 
		zap.String("uuid", uuid))

	ns.logger.Debug("🔥 SYMBOLICATOR: Waiting for channel availability")
	select {
	case ns.ch <- struct{}{}:
		ns.logger.Debug("🔥 SYMBOLICATOR: Acquired channel lock")
	case <-time.After(ns.timeout):
		ns.logger.Error("🔥 SYMBOLICATOR: Timeout waiting for channel", 
			zap.Duration("timeout", ns.timeout))
		return nil, fmt.Errorf("timeout")
	}

	defer func() {
		ns.logger.Debug("🔥 SYMBOLICATOR: Releasing channel lock")
		<-ns.ch
	}()

	ns.logger.Debug("🔥 SYMBOLICATOR: Checking cache for UUID", 
		zap.String("uuid", uuid))
	pm, ok := ns.cache.Get(uuid)

	if !ok {
		ns.logger.Info("🔥 SYMBOLICATOR: Cache miss, loading proguard mapping from store", 
			zap.String("uuid", uuid))

		pmf, err := ns.store.GetProguardMapping(ctx, uuid)

		if err != nil {
			ns.logger.Error("🔥 SYMBOLICATOR: Failed to get proguard mapping from store", 
				zap.Error(err),
				zap.String("uuid", uuid))
			return nil, err
		}

		ns.logger.Info("🔥 SYMBOLICATOR: Successfully loaded proguard mapping", 
			zap.String("uuid", uuid),
			zap.Int("size_bytes", len(pmf)))

		ns.logger.Debug("🔥 SYMBOLICATOR: Creating temp file for proguard mapping")
		f, err := os.CreateTemp("", "proguard-*.txt")

		if err != nil {
			ns.logger.Error("🔥 SYMBOLICATOR: Failed to create temp file", zap.Error(err))
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}

		defer f.Close()
		defer os.Remove(f.Name())

		ns.logger.Debug("🔥 SYMBOLICATOR: Writing proguard mapping to temp file", 
			zap.String("temp_file", f.Name()))

		_, err = f.Write(pmf)

		if err != nil {
			ns.logger.Error("🔥 SYMBOLICATOR: Failed to write to temp file", zap.Error(err))
			return nil, fmt.Errorf("failed to write proguard mapping to temp file: %w", err)
		}

		ns.logger.Debug("🔥 SYMBOLICATOR: Creating ProguardMapper from temp file")
		pm, err = symbolic.NewProguardMapper(f.Name())
		if err != nil {
			ns.logger.Error("🔥 SYMBOLICATOR: Failed to create ProguardMapper", 
				zap.Error(err),
				zap.String("temp_file", f.Name()))
			return nil, err
		}

		ns.logger.Info("🔥 SYMBOLICATOR: Successfully created ProguardMapper", 
			zap.String("uuid", uuid),
			zap.String("mapper_uuid", pm.UUID),
			zap.Bool("has_line_info", pm.HasLineInfo))

		// Test some sample remapping to see what happens
		ns.logger.Info("🔥 SYMBOLICATOR: Testing sample remapping...")
		
		// Try remapping a class that should exist in the mapping
		testClass, err := pm.RemapClass("androidx.activity.ComponentActivity")
		if err != nil {
			ns.logger.Warn("🔥 SYMBOLICATOR: Failed to remap test class", zap.Error(err))
		} else {
			ns.logger.Info("🔥 SYMBOLICATOR: Test class remap result",
				zap.String("input", "androidx.activity.ComponentActivity"),
				zap.String("output", testClass))
		}

		// Try remapping with an OBFUSCATED class name from the mapping file
		testObfuscated, err := pm.RemapClass("o2.c0")
		if err != nil {
			ns.logger.Warn("🔥 SYMBOLICATOR: Failed to remap obfuscated test class", zap.Error(err))
		} else {
			ns.logger.Info("🔥 SYMBOLICATOR: Obfuscated test class remap result",
				zap.String("input", "o2.c0"),
				zap.String("output", testObfuscated))
		}

		ns.logger.Info("🔥 SYMBOLICATOR: Adding ProguardMapper to cache", 
			zap.String("uuid", uuid))
		ns.cache.Add(uuid, pm)
	} else {
		ns.logger.Info("🔥 SYMBOLICATOR: Cache hit for UUID", 
			zap.String("uuid", uuid))
	}

	ns.logger.Info("🔥 SYMBOLICATOR: Calling RemapFrame", 
		zap.String("class", class),
		zap.String("method", method),
		zap.Int("line", line))

	result, err := pm.RemapFrame(class, method, line)
	if err != nil {
		ns.logger.Error("🔥 SYMBOLICATOR: RemapFrame failed", 
			zap.Error(err),
			zap.String("class", class),
			zap.String("method", method),
			zap.Int("line", line))
		return nil, err
	}

	ns.logger.Info("🔥 SYMBOLICATOR: RemapFrame succeeded", 
		zap.Int("result_frames", len(result)))

	// If no frames returned, let's debug why
	if len(result) == 0 {
		ns.logger.Warn("🔥 SYMBOLICATOR: RemapFrame returned 0 frames - debugging...",
			zap.String("input_class", class),
			zap.String("input_method", method),
			zap.Int("input_line", line),
			zap.String("mapper_uuid", pm.UUID),
			zap.Bool("mapper_has_line_info", pm.HasLineInfo))
		
		// Try just remapping the class to see if it exists
		remappedClass, err := pm.RemapClass(class)
		if err != nil {
			ns.logger.Warn("🔥 SYMBOLICATOR: RemapClass failed for input class", 
				zap.String("class", class), zap.Error(err))
		} else {
			ns.logger.Info("🔥 SYMBOLICATOR: RemapClass result for input class",
				zap.String("input_class", class),
				zap.String("remapped_class", remappedClass),
				zap.Bool("class_changed", class != remappedClass))
		}
	}

	// Add detailed logging of each result frame
	for i, frame := range result {
		ns.logger.Info("🔥 SYMBOLICATOR: RemapFrame result frame", 
			zap.Int("index", i),
			zap.String("input_class", class),
			zap.String("input_method", method),
			zap.Int("input_line", line),
			zap.String("output_class", frame.ClassName),
			zap.String("output_method", frame.MethodName),
			zap.Int("output_line", frame.LineNumber),
			zap.String("source_file", frame.SourceFile),
			zap.String("parameters", frame.ParameterNames))
	}

	return result, nil
}
