package symbolicatorprocessor

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

var (
	jsFile = "https://www.honeycomb.io/assets/js/basic-mapping.js"
	noFile = "https://www.honeycomb.io/assets/js/does-not-exist.js"
)

func TestSymbolicator(t *testing.T) {
	ctx := context.Background()

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalSourceMapConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)
	sym, _ := newBasicSymbolicator(ctx, 5*time.Second, 128, fs)

	sf, err := sym.symbolicate(ctx, 0, 34, "b", jsFile)
	line := formatStackFrame(sf)

	assert.NoError(t, err)
	assert.Equal(t, "    at bar(basic-mapping-original.js:8:1)", line)

	_, err = sym.symbolicate(ctx, 0, 34, "b", noFile)
	assert.Error(t, err)

	_, err = sym.symbolicate(ctx, math.MaxInt64, 34, "b", jsFile)
	assert.Error(t, err)

	_, err = sym.symbolicate(ctx, 0, math.MaxInt64, "b", jsFile)
	assert.Error(t, err)
}

func TestSymbolicatorCache(t *testing.T) {
	ctx := context.Background()

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalSourceMapConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)
	sym, _ := newBasicSymbolicator(ctx, 5*time.Second, 128, fs)

	// Cache should be empty to start
	assert.Equal(t, 0, sym.cache.Len())

	// First symbolication should add to cache
	sf, err := sym.symbolicate(ctx, 0, 34, "b", jsFile)
	line := formatStackFrame(sf)

	assert.NoError(t, err)
	assert.Equal(t, "    at bar(basic-mapping-original.js:8:1)", line)

	// Cache should have one entry
	assert.Equal(t, 1, sym.cache.Len())
}

func TestDSYMSymbolicator(t *testing.T) {
	ctx := context.Background()

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalSourceMapConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)
	sym, _ := newBasicSymbolicator(ctx, 5*time.Second, 128, fs)

	baseFrame := MetricKitCallStackFrame{
		BinaryUUID: "6A8CB813-45F6-3652-AD33-778FD1EAB196",
		OffsetIntoBinaryTextSegment: 100436,
		BinaryName: "chateaux-bufeaux",
	}
	sf, err := sym.symbolicateDSYMFrame(ctx, "cbx.app.dSYM", "Chateaux Bufeaux", baseFrame.BinaryUUID, baseFrame.OffsetIntoBinaryTextSegment)
	line := formatdSYMStackFrames(baseFrame, sf)

	assert.NoError(t, err)
	assert.Equal(t, "chateaux-bufeaux			0x18854 main() (/Users/mustafa/hny/chateaux-bufeaux-ios/Chateaux Bufeaux/Chateaux_BufeauxApp.swift:0) + 100372", line)

	// dSYM doesn't exist
	_, err = sym.symbolicateDSYMFrame(ctx, "other.app.dSYM", "Chateaux Bufeaux", baseFrame.BinaryUUID, baseFrame.OffsetIntoBinaryTextSegment)
	assert.Error(t, err)

	// binary doesn't exist in the dSYM
	_, err = sym.symbolicateDSYMFrame(ctx, "cbx.app.dSYM", "other binary", baseFrame.BinaryUUID, baseFrame.OffsetIntoBinaryTextSegment)
	assert.Error(t, err)

	// UUID isn't found in the binary
	_, err = sym.symbolicateDSYMFrame(ctx, "cbx.app.dSYM", "Chateaux Bufeaux", "2DBDCA05-2BAA-3BFE-9EF3-15A157D84058", baseFrame.OffsetIntoBinaryTextSegment)
	assert.Error(t, err)

	// // nothing at that offset
	_, err = sym.symbolicateDSYMFrame(ctx, "cbx.app.dSYM", "Chateaux Bufeaux", baseFrame.BinaryUUID, 9999999999)
	assert.Error(t, err)
}

func TestDSYMSymbolicatorCache(t *testing.T) {
	ctx := context.Background()

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalSourceMapConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)
	sym, _ := newBasicSymbolicator(ctx, 5*time.Second, 128, fs)

	// Cache should be empty to start
	assert.Equal(t, 0, sym.dsymCache.Len())

	// First symbolication should add to cache
	baseFrame := MetricKitCallStackFrame{
		BinaryUUID: "6A8CB813-45F6-3652-AD33-778FD1EAB196",
		OffsetIntoBinaryTextSegment: 100436,
		BinaryName: "chateaux-bufeaux",
	}
	sf, err := sym.symbolicateDSYMFrame(ctx, "cbx.app.dSYM", "Chateaux Bufeaux", baseFrame.BinaryUUID, baseFrame.OffsetIntoBinaryTextSegment)
	line := formatdSYMStackFrames(baseFrame, sf)

	assert.NoError(t, err)
	assert.Equal(t, "chateaux-bufeaux			0x18854 main() (/Users/mustafa/hny/chateaux-bufeaux-ios/Chateaux Bufeaux/Chateaux_BufeauxApp.swift:0) + 100372", line)

	// Cache should have one entry
	assert.Equal(t, 1, sym.dsymCache.Len())
}
