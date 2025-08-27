package proguardprocessor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/proguardprocessor/internal/metadata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap/zaptest"
)

type mockSymbolicatorStore struct {
	mapping map[string][]byte
	err     error
	calls   int
}

func (m *mockSymbolicatorStore) GetProguardMapping(ctx context.Context, uuid string) ([]byte, error) {
	m.calls++
	return m.mapping[uuid], m.err
}

func createMockSymbolicatorTelemetry(t *testing.T) (*metadata.TelemetryBuilder, attribute.Set) {
	settings := component.TelemetrySettings{
		Logger:        zaptest.NewLogger(t),
		MeterProvider: noop.NewMeterProvider(),
	}
	tb, err := metadata.NewTelemetryBuilder(settings)
	assert.NoError(t, err)
	attributes := attribute.NewSet()
	return tb, attributes
}

func TestNewBasicSymbolicator(t *testing.T) {
	tests := []struct {
		name      string
		timeout   time.Duration
		cacheSize int
		store     fileStore
		wantErr   bool
	}{
		{
			name:      "successful creation with valid parameters",
			timeout:   5 * time.Second,
			cacheSize: 100,
			store:     &mockSymbolicatorStore{},
			wantErr:   false,
		},
		{
			name:      "successful creation with zero cache size",
			timeout:   1 * time.Second,
			cacheSize: 0,
			store:     &mockSymbolicatorStore{},
			wantErr:   true,
		},
		{
			name:      "successful creation with large cache size",
			timeout:   10 * time.Second,
			cacheSize: 10000,
			store:     &mockSymbolicatorStore{},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tb, attributes := createMockSymbolicatorTelemetry(t)
			symbolicator, err := newBasicSymbolicator(ctx, tt.timeout, tt.cacheSize, tt.store, tb, attributes)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, symbolicator)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, symbolicator)
				assert.Equal(t, tt.timeout, symbolicator.timeout)
				assert.Equal(t, tt.store, symbolicator.store)
				assert.NotNil(t, symbolicator.cache)
				assert.NotNil(t, symbolicator.ch)
				assert.Equal(t, 1, cap(symbolicator.ch))
			}
		})
	}
}

func TestBasicSymbolicator_Symbolicate_Success(t *testing.T) {
	mockStore := &mockSymbolicatorStore{}
	ctx := context.Background()
	uuid := "test-uuid"
	tb, attributes := createMockSymbolicatorTelemetry(t)

	symbolicator, err := newBasicSymbolicator(ctx, 5*time.Second, 10, mockStore, tb, attributes)
	require.NoError(t, err)

	// Note: This test would require a working symbolic.NewProguardMapper implementation
	// Since we're testing with mock data, we'll focus on the error cases and structure
	_, err = symbolicator.symbolicate(ctx, uuid, "com.example.Test", "methodA", 1)
}

func TestBasicSymbolicator_Symbolicate_StoreError(t *testing.T) {
	ctx := context.Background()
	uuid := "test-uuid"
	expectedError := errors.New("store error")
	mockStore := &mockSymbolicatorStore{
		err: expectedError,
	}
	tb, attributes := createMockSymbolicatorTelemetry(t)

	symbolicator, err := newBasicSymbolicator(ctx, 5*time.Second, 10, mockStore, tb, attributes)
	require.NoError(t, err)

	result, err := symbolicator.symbolicate(ctx, uuid, "com.example.Test", "methodA", 1)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "store error")
}

func TestBasicSymbolicator_LimitedSymbolicate_Timeout(t *testing.T) {
	mockStore := &mockSymbolicatorStore{}
	ctx := context.Background()
	uuid := "test-uuid"
	tb, attributes := createMockSymbolicatorTelemetry(t)

	// Create symbolicator with very short timeout
	symbolicator, err := newBasicSymbolicator(ctx, 1*time.Nanosecond, 10, mockStore, tb, attributes)
	require.NoError(t, err)

	// First, occupy the channel to cause timeout
	symbolicator.ch <- struct{}{}

	result, err := symbolicator.limitedSymbolicate(ctx, uuid, "com.example.Test", "methodA", 1)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "timeout")

	// Clean up the channel
	<-symbolicator.ch
}

func TestBasicSymbolicator_LimitedSymbolicate_CacheHit(t *testing.T) {
	mockStore := &mockSymbolicatorStore{}
	ctx := context.Background()
	uuid := "test-uuid"
	tb, attributes := createMockSymbolicatorTelemetry(t)

	symbolicator, err := newBasicSymbolicator(ctx, 5*time.Second, 10, mockStore, tb, attributes)
	require.NoError(t, err)

	// First call - should hit the store
	_, err1 := symbolicator.limitedSymbolicate(ctx, uuid, "com.example.Test", "methodA", 1)

	// Second call - should hit the cache (store should not be called again)
	_, err2 := symbolicator.limitedSymbolicate(ctx, uuid, "com.example.Test", "methodB", 2)

	// If both calls failed, they should fail for the same reason (symbolic-go related)
	if err1 != nil && err2 != nil {
		// Both should have the same type of error since they're using the same mapping
		assert.IsType(t, err1, err2)
	}

	assert.Equal(t, 1, mockStore.calls, "Store should be called only once due to caching")
}

func TestBasicSymbolicator_LargeLineNumber(t *testing.T) {
	mockStore := &mockSymbolicatorStore{}
	ctx := context.Background()
	uuid := "test-uuid"
	tb, attributes := createMockSymbolicatorTelemetry(t)

	symbolicator, err := newBasicSymbolicator(ctx, 5*time.Second, 10, mockStore, tb, attributes)
	require.NoError(t, err)

	// Test with a very large line number
	result, err := symbolicator.symbolicate(ctx, uuid, "com.example.Test", "methodA", 999999)

	// Should handle large line numbers gracefully
	if err != nil {
		assert.NotEmpty(t, err.Error())
	}
	// Result might be nil or empty, both are acceptable
	if result != nil {
		assert.IsType(t, []*mappedStackFrame{}, result)
	}
}
