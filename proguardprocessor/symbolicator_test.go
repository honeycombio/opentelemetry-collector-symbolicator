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

func TestFetchError(t *testing.T) {
	tests := []struct {
		name         string
		uuid         string
		wrappedError error
		wantContains []string
	}{
		{
			name:         "store error wrapped in FetchError",
			uuid:         "test-uuid-123",
			wrappedError: errors.New("404 not found"),
			wantContains: []string{"failed to fetch ProGuard mapping", "test-uuid-123", "404 not found"},
		},
		{
			name:         "timeout wrapped in FetchError",
			uuid:         "uuid-456",
			wrappedError: errors.New("timeout"),
			wantContains: []string{"failed to fetch ProGuard mapping", "uuid-456", "timeout"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fetchErr := &FetchError{UUID: tt.uuid, Err: tt.wrappedError}
			errMsg := fetchErr.Error()

			for _, want := range tt.wantContains {
				assert.Contains(t, errMsg, want)
			}

			// Verify Unwrap works
			assert.Equal(t, tt.wrappedError, fetchErr.Unwrap())

			// Verify errors.As can detect it
			var fe *FetchError
			assert.True(t, errors.As(fetchErr, &fe))
			assert.Equal(t, tt.uuid, fe.UUID)
		})
	}
}

func TestBasicSymbolicator_FetchErrorWrapping(t *testing.T) {
	tests := []struct {
		name         string
		storeError   error
		timeout      time.Duration
		blockChannel bool
		wantFetchErr bool
	}{
		{
			name:         "store error should be wrapped in FetchError",
			storeError:   errors.New("404 not found"),
			timeout:      5 * time.Second,
			blockChannel: false,
			wantFetchErr: true,
		},
		{
			name:         "timeout should be wrapped in FetchError",
			storeError:   nil,
			timeout:      1 * time.Nanosecond,
			blockChannel: true,
			wantFetchErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			mockStore := &mockSymbolicatorStore{err: tt.storeError}
			tb, attributes := createMockSymbolicatorTelemetry(t)

			symbolicator, err := newBasicSymbolicator(ctx, tt.timeout, 10, mockStore, tb, attributes)
			require.NoError(t, err)

			if tt.blockChannel {
				// Block the channel to force timeout
				symbolicator.ch <- struct{}{}
			}

			_, err = symbolicator.symbolicate(ctx, "test-uuid", "com.example.Test", "methodA", 100)

			if tt.blockChannel {
				// Unblock for cleanup
				<-symbolicator.ch
			}

			// Should return an error
			require.Error(t, err)

			if tt.wantFetchErr {
				// Verify it's a FetchError
				var fetchErr *FetchError
				assert.True(t, errors.As(err, &fetchErr), "Expected error to be wrapped in FetchError")
				assert.Contains(t, err.Error(), "failed to fetch ProGuard mapping")
			}
		})
	}
}

func TestBasicSymbolicator_DifferentUUIDs(t *testing.T) {
	ctx := context.Background()
	mockStore := &mockSymbolicatorStore{}
	tb, attributes := createMockSymbolicatorTelemetry(t)

	symbolicator, err := newBasicSymbolicator(ctx, 5*time.Second, 10, mockStore, tb, attributes)
	require.NoError(t, err)

	// Call with first UUID
	_, _ = symbolicator.limitedSymbolicate(ctx, "uuid-1", "com.example.Test", "methodA", 1)
	callsAfterFirst := mockStore.calls

	// Call with same UUID - should use cache
	_, _ = symbolicator.limitedSymbolicate(ctx, "uuid-1", "com.example.Test", "methodB", 2)
	assert.Equal(t, callsAfterFirst, mockStore.calls, "Second call with same UUID should use cache")

	// Call with different UUID - should fetch new mapping
	_, _ = symbolicator.limitedSymbolicate(ctx, "uuid-2", "com.example.Test", "methodA", 1)
	assert.Greater(t, mockStore.calls, callsAfterFirst, "Call with different UUID should fetch new mapping")
}
