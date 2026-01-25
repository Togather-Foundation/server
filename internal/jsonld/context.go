package jsonld

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	DefaultContextVersion = "v0.1"
	DefaultContextDir     = "contexts/sel"
)

var (
	ErrContextNotFound = errors.New("context not found")
	ErrInvalidVersion  = errors.New("invalid context version")
)

// ContextLoader loads versioned JSON-LD contexts with in-memory caching.
type ContextLoader struct {
	mu      sync.RWMutex
	cache   map[string]map[string]any
	baseDir string
}

// NewContextLoader creates a loader rooted at baseDir. If baseDir is empty, DefaultContextDir is used.
func NewContextLoader(baseDir string) *ContextLoader {
	if baseDir == "" {
		baseDir = resolveDefaultContextDir()
	}
	return &ContextLoader{
		cache:   make(map[string]map[string]any),
		baseDir: baseDir,
	}
}

// Load returns the JSON-LD context document for the requested version.
func (l *ContextLoader) Load(version string) (map[string]any, error) {
	if version == "" {
		return nil, ErrInvalidVersion
	}

	l.mu.RLock()
	if cached, ok := l.cache[version]; ok {
		l.mu.RUnlock()
		return cached, nil
	}
	l.mu.RUnlock()

	path := filepath.Join(l.baseDir, fmt.Sprintf("%s.jsonld", version))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrContextNotFound
		}
		return nil, err
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	l.mu.Lock()
	l.cache[version] = doc
	l.mu.Unlock()

	return doc, nil
}

// LoadContext loads the specified context version using the default loader.
func LoadContext(version string) (map[string]any, error) {
	return defaultLoader.Load(version)
}

// LoadDefaultContext loads the default context version.
func LoadDefaultContext() (map[string]any, error) {
	return defaultLoader.Load(DefaultContextVersion)
}

var defaultLoader = NewContextLoader("")

func resolveDefaultContextDir() string {
	if existsContextDir(DefaultContextDir) {
		return DefaultContextDir
	}

	cwd, err := os.Getwd()
	if err != nil {
		return DefaultContextDir
	}

	for i := 0; i < 10; i++ {
		candidate := filepath.Join(cwd, DefaultContextDir)
		if existsContextDir(candidate) {
			return candidate
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}

	return DefaultContextDir
}

func existsContextDir(dir string) bool {
	path := filepath.Join(dir, fmt.Sprintf("%s.jsonld", DefaultContextVersion))
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
