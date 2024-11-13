package symbolicatorprocessor

import (
	"context"
	"fmt"
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

func (fs *fileStore) GetSourceMap(ctx context.Context, url string) (string, string, error) {
	source, err := os.ReadFile(filepath.Join(fs.root, url))

	if err != nil {
		return "", "", fmt.Errorf("%w: %s", errFailedToFindSourceFile, url)
	}

	fs.logger.Info("Found source file", zap.String("path", filepath.Join(fs.root, url)))

	matches := mappingURLRegex.FindStringSubmatch(string(source))

	if len(matches) <= 0 {
		return string(source), "", fmt.Errorf("%w: %s", errFailedToFindSourceMapLocation, url)
	}

	// the capture group we want is the last one
	mapName := matches[len(matches)-1]

	sourceMap, err := os.ReadFile(filepath.Join(fs.root, mapName))

	if err != nil {
		return string(source), "", fmt.Errorf("%w: %s", errFailedToFindSourceMap, mapName)
	}

	fs.logger.Info("Found map file", zap.String("path", filepath.Join(fs.root, mapName)))

	return string(source), string(sourceMap), nil
}
