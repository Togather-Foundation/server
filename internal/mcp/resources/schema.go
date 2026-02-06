package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"sigs.k8s.io/yaml"
)

const (
	schemaMIMEType     = "application/json"
	openAPIResource    = "schema://openapi"
	serverInfoResource = "info://server"
)

type ServerCapabilities struct {
	Tools     bool `json:"tools"`
	Resources bool `json:"resources"`
	Prompts   bool `json:"prompts"`
}

type ServerInfo struct {
	Name         string             `json:"name"`
	Version      string             `json:"version,omitempty"`
	BaseURL      string             `json:"base_url,omitempty"`
	Capabilities ServerCapabilities `json:"capabilities"`
	Transport    string             `json:"transport,omitempty"`
}

type SchemaResources struct {
	openAPIPath string // Path to OpenAPI YAML file

	openAPIOnce sync.Once
	openAPIJSON string
	openAPIErr  error

	infoOnce sync.Once
	infoJSON string
	infoErr  error
}

// NewSchemaResources creates a new schema resources handler.
// openAPIPath specifies where to find the OpenAPI YAML file.
func NewSchemaResources(openAPIPath string) *SchemaResources {
	if openAPIPath == "" {
		openAPIPath = "specs/001-sel-backend/contracts/openapi.yaml" // Default fallback
	}
	return &SchemaResources{
		openAPIPath: openAPIPath,
	}
}

func (r *SchemaResources) OpenAPIResource() mcp.Resource {
	return mcp.NewResource(
		openAPIResource,
		"OpenAPI Schema",
		mcp.WithResourceDescription("OpenAPI specification for the Togather SEL backend"),
		mcp.WithMIMEType(schemaMIMEType),
	)
}

func (r *SchemaResources) InfoResource() mcp.Resource {
	return mcp.NewResource(
		serverInfoResource,
		"Server Info",
		mcp.WithResourceDescription("MCP server metadata and capabilities"),
		mcp.WithMIMEType(schemaMIMEType),
	)
}

func (r *SchemaResources) OpenAPIReadHandler() func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(_ context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		content, err := r.loadOpenAPI()
		if err != nil {
			return nil, err
		}

		responseURI := openAPIResource
		if request.Params.URI != "" {
			responseURI = request.Params.URI
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      responseURI,
				MIMEType: schemaMIMEType,
				Text:     content,
			},
		}, nil
	}
}

func (r *SchemaResources) InfoReadHandler(info ServerInfo) func(context.Context, mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	return func(_ context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		content, err := r.loadInfo(info)
		if err != nil {
			return nil, err
		}

		responseURI := serverInfoResource
		if request.Params.URI != "" {
			responseURI = request.Params.URI
		}

		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      responseURI,
				MIMEType: schemaMIMEType,
				Text:     content,
			},
		}, nil
	}
}

func (r *SchemaResources) loadOpenAPI() (string, error) {
	r.openAPIOnce.Do(func() {
		// Try configured path first
		data, err := os.ReadFile(r.openAPIPath)
		if err != nil {
			// Fallback: try resolving from repository root
			data, err = os.ReadFile(r.resolveOpenAPIPath())
			if err != nil {
				r.openAPIErr = err
				return
			}
		}

		jsonData, err := yaml.YAMLToJSON(data)
		if err != nil {
			r.openAPIErr = err
			return
		}
		r.openAPIJSON = string(jsonData)
	})

	if r.openAPIErr != nil {
		return "", fmt.Errorf("load openapi: %w", r.openAPIErr)
	}

	return r.openAPIJSON, nil
}

func (r *SchemaResources) resolveOpenAPIPath() string {
	root, err := repoRoot()
	if err != nil {
		return r.openAPIPath
	}
	return filepath.Join(root, r.openAPIPath)
}

func (r *SchemaResources) loadInfo(info ServerInfo) (string, error) {
	r.infoOnce.Do(func() {
		data, err := json.Marshal(info)
		if err != nil {
			r.infoErr = err
			return
		}
		r.infoJSON = string(data)
	})

	if r.infoErr != nil {
		return "", fmt.Errorf("load server info: %w", r.infoErr)
	}

	return r.infoJSON, nil
}

func repoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd, nil
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", os.ErrNotExist
	}
	base := filepath.Dir(file)
	return filepath.Join(base, "..", "..", ".."), nil
}
