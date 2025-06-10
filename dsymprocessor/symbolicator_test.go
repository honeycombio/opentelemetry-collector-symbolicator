package dsymprocessor

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestDSYMSymbolicator(t *testing.T) {
	ctx := context.Background()

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalDSYMConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)
	sym, _ := newBasicSymbolicator(ctx, 5*time.Second, 128, fs)

	baseFrame := MetricKitCallStackFrame{
		BinaryUUID: "6A8CB813-45F6-3652-AD33-778FD1EAB196",
		OffsetIntoBinaryTextSegment: 100436,
		BinaryName: "chateaux-bufeaux",
	}
	sf, err := sym.symbolicateFrame(ctx, baseFrame.BinaryUUID, "Chateaux Bufeaux", baseFrame.OffsetIntoBinaryTextSegment)
	line := formatStackFrames(baseFrame, sf)

	assert.NoError(t, err)
	assert.Equal(t, "chateaux-bufeaux			0x18854 main() (/Users/mustafa/hny/chateaux-bufeaux-ios/Chateaux Bufeaux/Chateaux_BufeauxApp.swift:0) + 100372", line)

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

	fs, err := newFileStore(ctx, zaptest.NewLogger(t), &LocalDSYMConfiguration{Path: "../test_assets"})
	assert.NoError(t, err)
	sym, _ := newBasicSymbolicator(ctx, 5*time.Second, 128, fs)

	// Cache should be empty to start
	assert.Equal(t, 0, sym.cache.Len())

	// First symbolication should add to cache
	baseFrame := MetricKitCallStackFrame{
		BinaryUUID: "6A8CB813-45F6-3652-AD33-778FD1EAB196",
		OffsetIntoBinaryTextSegment: 100436,
		BinaryName: "chateaux-bufeaux",
	}
	sf, err := sym.symbolicateFrame(ctx, baseFrame.BinaryUUID, "Chateaux Bufeaux", baseFrame.OffsetIntoBinaryTextSegment)
	line := formatStackFrames(baseFrame, sf)

	assert.NoError(t, err)
	assert.Equal(t, "chateaux-bufeaux			0x18854 main() (/Users/mustafa/hny/chateaux-bufeaux-ios/Chateaux Bufeaux/Chateaux_BufeauxApp.swift:0) + 100372", line)

	// Cache should have one entry
	assert.Equal(t, 1, sym.cache.Len())
}
