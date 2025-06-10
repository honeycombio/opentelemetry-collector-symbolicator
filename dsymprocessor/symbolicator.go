package dsymprocessor

import (
	"context"
	"fmt"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/honeycombio/symbolic-go"
)

type dsymStore interface {
	GetDSYM(ctx context.Context, debugId, binaryName string) ([]byte, error)
}

type basicSymbolicator struct {
	store   dsymStore
	timeout time.Duration
	ch      chan struct{}
	cache *lru.Cache[string, *symbolic.Archive]
}

func newBasicSymbolicator(_ context.Context, timeout time.Duration, cacheSize int, store dsymStore) (*basicSymbolicator, error) {
	cache, err := lru.New[string, *symbolic.Archive](cacheSize)
	if err != nil {
		return nil, err
	}

	return &basicSymbolicator{
		store:   store,
		timeout: timeout,
		// the channel is buffered to allow for a single request to be in progress at a time
		ch:    make(chan struct{}, 1),
		cache: cache,
	}, nil
}

type mappedDSYMStackFrame struct {
	path string
	instrAddr uint64
	lang string
	line uint32
	symAddr uint64
	symbol string
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
	archive, ok := ns.cache.Get(cacheKey)

	if !ok {
		dSYMbytes, err := ns.store.GetDSYM(ctx, debugId, binaryName)
		if err != nil {
			return nil, err
		}
		archive, err = symbolic.NewArchiveFromBytes(dSYMbytes)

		if err != nil {
			return nil, err
		}

		ns.cache.Add(cacheKey, archive)
	}

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
	for i,loc := range(locations) {
		res[i] = &mappedDSYMStackFrame{
			path: loc.FullPath,
			instrAddr: loc.InstrAddr,
			lang: loc.Lang,
			line: loc.Line,
			symAddr: loc.SymAddr,
			symbol: loc.Symbol,
		}
	}
	return res, nil
}
