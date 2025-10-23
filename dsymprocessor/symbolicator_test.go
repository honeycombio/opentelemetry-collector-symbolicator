package dsymprocessor

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/dsymprocessor/internal/metadata"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap/zaptest"
)

// mockDSYMStore is a mock implementation of dsymStore for testing
type mockDSYMStore struct {
	callCount map[string]int
	data      map[string][]byte // key -> data mapping for files that exist
	mu        sync.Mutex
}

func newMockDSYMStore() *mockDSYMStore {
	return &mockDSYMStore{
		callCount: make(map[string]int),
		data:      make(map[string][]byte),
	}
}

func (m *mockDSYMStore) AddFile(debugId, binaryName string, data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := debugId + "/" + binaryName
	m.data[key] = data
}

func (m *mockDSYMStore) GetDSYM(ctx context.Context, debugId, binaryName string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := debugId + "/" + binaryName
	m.callCount[key]++

	if data, exists := m.data[key]; exists {
		return data, nil
	}

	return nil, errFailedToFindDSYM
}

func (m *mockDSYMStore) GetCallCount(debugId, binaryName string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := debugId + "/" + binaryName
	return m.callCount[key]
}

// callCountingStore wraps a dsymStore and counts how many times GetDSYM is called per unique key
type callCountingStore struct {
	wrapped   dsymStore
	callCount map[string]int
	mu        sync.Mutex
}

func newCallCountingStore(wrapped dsymStore) *callCountingStore {
	return &callCountingStore{
		wrapped:   wrapped,
		callCount: make(map[string]int),
	}
}

func (c *callCountingStore) GetDSYM(ctx context.Context, debugId, binaryName string) ([]byte, error) {
	c.mu.Lock()
	key := debugId + "/" + binaryName
	c.callCount[key]++
	currentCount := c.callCount[key]
	c.mu.Unlock()

	fmt.Printf("[STORE CALL #%d] GetDSYM(%s, %s)\n", currentCount, debugId, binaryName)

	return c.wrapped.GetDSYM(ctx, debugId, binaryName)
}

func (c *callCountingStore) GetCallCount(debugId, binaryName string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := debugId + "/" + binaryName
	return c.callCount[key]
}

func TestDSYMSymbolicator(t *testing.T) {
	ctx := context.Background()

	testTel := componenttest.NewTelemetry()
	tb, err := metadata.NewTelemetryBuilder(testTel.NewTelemetrySettings())
	assert.NoError(t, err)
	defer tb.Shutdown()

	attributes := attribute.NewSet(
		attribute.String("processor_type", "dsym_symbolicator"),
	)

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalDSYMConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)
	sym, _ := newBasicSymbolicator(ctx, 5*time.Second, 128, fs, tb, attributes)

	baseFrame := MetricKitCallStackFrame{
		BinaryUUID:                  "6A8CB813-45F6-3652-AD33-778FD1EAB196",
		OffsetIntoBinaryTextSegment: 100436,
		BinaryName:                  "chateaux-bufeaux",
	}
	sf, err := sym.symbolicateFrame(ctx, baseFrame.BinaryUUID, "Chateaux Bufeaux", baseFrame.OffsetIntoBinaryTextSegment)
	line := formatMetricKitStackFrames(baseFrame, sf)

	assert.NoError(t, err)
	assert.Equal(t, "chateaux-bufeaux			0x18854 main (/Users/mustafa/hny/chateaux-bufeaux-ios/Chateaux Bufeaux/Chateaux_BufeauxApp.swift:0) + 100372", line)

	// UUID doesn't exist
	_, err = sym.symbolicateFrame(ctx, "2DBDCA05-2BAA-3BFE-9EF3-15A157D84058", "Chateaux Bufeaux", baseFrame.OffsetIntoBinaryTextSegment)
	assert.Error(t, err)

	// binary doesn't exist in the dSYM
	_, err = sym.symbolicateFrame(ctx, baseFrame.BinaryUUID, "other binary", baseFrame.OffsetIntoBinaryTextSegment)
	assert.Error(t, err)

	// // nothing at that offset
	_, err = sym.symbolicateFrame(ctx, baseFrame.BinaryUUID, "Chateaux Bufeaux", 9999999999)
	assert.Error(t, err)
}

func TestDSYMSymbolicatorCache(t *testing.T) {
	ctx := context.Background()

	testTel := componenttest.NewTelemetry()
	tb, err := metadata.NewTelemetryBuilder(testTel.NewTelemetrySettings())
	assert.NoError(t, err)
	defer tb.Shutdown()

	attributes := attribute.NewSet(
		attribute.String("processor_type", "dsym_symbolicator"),
	)

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalDSYMConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)
	sym, _ := newBasicSymbolicator(ctx, 5*time.Second, 128, fs, tb, attributes)

	// Cache should be empty to start
	assert.Equal(t, 0, sym.cache.Len())

	// First symbolication should add to cache
	baseFrame := MetricKitCallStackFrame{
		BinaryUUID:                  "6A8CB813-45F6-3652-AD33-778FD1EAB196",
		OffsetIntoBinaryTextSegment: 100436,
		BinaryName:                  "chateaux-bufeaux",
	}
	sf, err := sym.symbolicateFrame(ctx, baseFrame.BinaryUUID, "Chateaux Bufeaux", baseFrame.OffsetIntoBinaryTextSegment)
	line := formatMetricKitStackFrames(baseFrame, sf)

	assert.NoError(t, err)
	assert.Equal(t, "chateaux-bufeaux			0x18854 main (/Users/mustafa/hny/chateaux-bufeaux-ios/Chateaux Bufeaux/Chateaux_BufeauxApp.swift:0) + 100372", line)

	// Cache should have one entry
	assert.Equal(t, 1, sym.cache.Len())
}

// TestNegativeCaching_Unit tests negative caching with a mock store
func TestNegativeCaching_Unit(t *testing.T) {
	ctx := context.Background()

	testTel := componenttest.NewTelemetry()
	tb, err := metadata.NewTelemetryBuilder(testTel.NewTelemetrySettings())
	assert.NoError(t, err)
	defer tb.Shutdown()

	attributes := attribute.NewSet(
		attribute.String("processor_type", "dsym_symbolicator"),
	)

	mockStore := newMockDSYMStore()
	sym, err := newBasicSymbolicator(ctx, 5*time.Second, 128, mockStore, tb, attributes)
	assert.NoError(t, err)

	t.Run("missing file is only fetched once", func(t *testing.T) {
		missingUUID := "MISSING-UUID"
		missingBinary := "MissingBinary"

		// Request missing file 5 times
		for i := 0; i < 5; i++ {
			_, err := sym.symbolicateFrame(ctx, missingUUID, missingBinary, 12345)
			assert.Error(t, err, "Should return error for missing file")
		}

		// Store should only be called once, then negative cached
		assert.Equal(t, 1, mockStore.GetCallCount(missingUUID, missingBinary))
	})

	t.Run("different missing files are tracked separately", func(t *testing.T) {
		// Request two different missing files, each 3 times
		for i := 0; i < 3; i++ {
			_, err := sym.symbolicateFrame(ctx, "UUID-1", "Binary1", 12345)
			assert.Error(t, err)
		}

		for i := 0; i < 3; i++ {
			_, err := sym.symbolicateFrame(ctx, "UUID-2", "Binary2", 12345)
			assert.Error(t, err)
		}

		// Each unique file should only hit the store once
		assert.Equal(t, 1, mockStore.GetCallCount("UUID-1", "Binary1"))
		assert.Equal(t, 1, mockStore.GetCallCount("UUID-2", "Binary2"))
	})
}

// TestNegativeCaching_ErrorMessages tests that cached errors have appropriate messages
func TestNegativeCaching_ErrorMessages(t *testing.T) {
	ctx := context.Background()

	testTel := componenttest.NewTelemetry()
	tb, err := metadata.NewTelemetryBuilder(testTel.NewTelemetrySettings())
	assert.NoError(t, err)
	defer tb.Shutdown()

	attributes := attribute.NewSet(
		attribute.String("processor_type", "dsym_symbolicator"),
	)

	mockStore := newMockDSYMStore()
	sym, err := newBasicSymbolicator(ctx, 5*time.Second, 128, mockStore, tb, attributes)
	assert.NoError(t, err)

	missingUUID := "MISSING-UUID"
	missingBinary := "MissingBinary"

	// First request - should hit store and get real error
	_, err1 := sym.symbolicateFrame(ctx, missingUUID, missingBinary, 12345)
	assert.Error(t, err1)
	assert.Contains(t, err1.Error(), "failed to find dSYM")

	// Second request - should be from negative cache
	_, err2 := sym.symbolicateFrame(ctx, missingUUID, missingBinary, 12345)
	assert.Error(t, err2)
	assert.Contains(t, err2.Error(), "not found (cached)")

	// Verify store was only called once
	assert.Equal(t, 1, mockStore.GetCallCount(missingUUID, missingBinary))
}

// TestNegativeCaching_Integration verifies negative caching works with real file store
func TestNegativeCaching_Integration(t *testing.T) {
	ctx := context.Background()

	testTel := componenttest.NewTelemetry()
	tb, err := metadata.NewTelemetryBuilder(testTel.NewTelemetrySettings())
	assert.NoError(t, err)
	defer tb.Shutdown()

	attributes := attribute.NewSet(
		attribute.String("processor_type", "dsym_symbolicator"),
	)

	// Create file store with test assets
	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalDSYMConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)

	// Wrap it with call counter
	countingStore := newCallCountingStore(fs)
	sym, _ := newBasicSymbolicator(ctx, 5*time.Second, 128, countingStore, tb, attributes)

	missingUUID := "AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE"
	missingBinary := "NonExistentBinary"

	// Request the same missing file 10 times
	for i := 0; i < 10; i++ {
		_, err := sym.symbolicateFrame(ctx, missingUUID, missingBinary, 12345)
		assert.Error(t, err, "Should error when file doesn't exist")
	}

	// Store should only be called once due to negative caching
	assert.Equal(t, 1, countingStore.GetCallCount(missingUUID, missingBinary),
		"Store should only be called once, then failures are cached")
}

// TestNegativeCaching_MixedFiles verifies both positive and negative caching work together
func TestNegativeCaching_MixedFiles(t *testing.T) {
	ctx := context.Background()

	testTel := componenttest.NewTelemetry()
	tb, err := metadata.NewTelemetryBuilder(testTel.NewTelemetrySettings())
	assert.NoError(t, err)
	defer tb.Shutdown()

	attributes := attribute.NewSet(
		attribute.String("processor_type", "dsym_symbolicator"),
	)

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalDSYMConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)

	countingStore := newCallCountingStore(fs)
	sym, _ := newBasicSymbolicator(ctx, 5*time.Second, 128, countingStore, tb, attributes)

	// Valid file that exists
	existingUUID := "6A8CB813-45F6-3652-AD33-778FD1EAB196"
	existingBinary := "Chateaux Bufeaux"

	// Missing file
	missingUUID := "MISSING-UUID-1234"
	missingBinary := "MissingBinary"

	// Request existing file 5 times
	for i := 0; i < 5; i++ {
		_, err := sym.symbolicateFrame(ctx, existingUUID, existingBinary, 100436)
		assert.NoError(t, err)
	}

	// Request missing file 5 times
	for i := 0; i < 5; i++ {
		_, err := sym.symbolicateFrame(ctx, missingUUID, missingBinary, 12345)
		assert.Error(t, err)
	}

	// Both should be cached after first call
	assert.Equal(t, 1, countingStore.GetCallCount(existingUUID, existingBinary),
		"Existing files should be cached")
	assert.Equal(t, 1, countingStore.GetCallCount(missingUUID, missingBinary),
		"Missing files should be negative cached")
}
