package symbolicatorprocessor

import (
	"context"
	"fmt"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"

	"go.uber.org/zap"
)

var (
	mappingURLRegex                  = regexp.MustCompile(`\/\/[#@]\s(source(?:Mapping)?URL)=\s*(\S+)`)
	errFailedToFindSourceFile        = fmt.Errorf("failed to find source file")
	errFailedToFindSourceMapLocation = fmt.Errorf("failed to find source map location")
	errFailedToFindSourceMap         = fmt.Errorf("failed to find source map")
)

type fileStore struct {
	logger *zap.Logger
	root   string
}

func newFileStore(root string, logger *zap.Logger) *fileStore {
	return &fileStore{
		logger: logger,
		root:   root,
	}
}

func (fs *fileStore) GetSourceMap(ctx context.Context, url string) ([]byte, []byte, error) {
	u, err := neturl.Parse(url)

	if err != nil {
		return nil, nil, err
	}

	path := filepath.Join(fs.root, u.Path)

	source, err := os.ReadFile(path)

	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", errFailedToFindSourceFile, path)
	}

	fs.logger.Info("Found source file", zap.String("path", path))

	matches := mappingURLRegex.FindSubmatch(source)

	if len(matches) <= 0 {
		return nil, nil, fmt.Errorf("%w: %s", errFailedToFindSourceMapLocation, path)
	}

	// the capture group we want is the last one
	mapName := string(matches[len(matches)-1])

	// the map name is relative to the source file
	path = filepath.Join(filepath.Dir(path), mapName)

	sourceMap, err := os.ReadFile(path)

	if err != nil {
		return nil, nil, fmt.Errorf("%w: %s", errFailedToFindSourceMap, mapName)
	}

	fs.logger.Info("Found map file", zap.String("path", path))

	return source, sourceMap, nil
}
