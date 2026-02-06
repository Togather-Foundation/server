package resources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
)

const (
	contextMIMEType  = "application/ld+json"
	contextResource  = "context://%s"
	contextDirectory = "contexts"
)

type ContextResources struct {
	mu    sync.RWMutex
	cache map[string]string
}

func NewContextResources() *ContextResources {
	return &ContextResources{
		cache: make(map[string]string),
	}
}

func (r *ContextResources) Resource(uri, name, description string) mcp.Resource {
	return mcp.NewResource(
		uri,
		name,
		mcp.WithResourceDescription(description),
		mcp.WithMIMEType(contextMIMEType),
	)
}

func (r *ContextResources) ReadHandler(filePath, uri string) func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(_ context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		content, err := r.loadFile(filePath)
		if err != nil {
			return nil, err
		}

		responseURI := uri
		if request.Params.URI != "" {
			responseURI = request.Params.URI
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      responseURI,
				MIMEType: contextMIMEType,
				Text:     content,
			},
		}, nil
	}
}

func (r *ContextResources) loadFile(path string) (string, error) {
	r.mu.RLock()
	if cached, ok := r.cache[path]; ok {
		r.mu.RUnlock()
		return cached, nil
	}
	r.mu.RUnlock()

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read context %s: %w", path, err)
	}

	r.mu.Lock()
	r.cache[path] = string(content)
	r.mu.Unlock()

	return string(content), nil
}

func ContextPath(rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(contextDirectory, rel)
}

func ContextURI(name string) string {
	return fmt.Sprintf(contextResource, name)
}
