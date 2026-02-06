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
	openAPISourcePath  = "specs/001-sel-backend/contracts/openapi.yaml"
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
	openAPIOnce sync.Once
	openAPIJSON string
	openAPIErr  error

	infoOnce sync.Once
	infoJSON string
	infoErr  error
}

func NewSchemaResources() *SchemaResources {
	return &SchemaResources{}
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
		data, err := os.ReadFile(openAPISourcePath)
		if err != nil {
			data, err = os.ReadFile(resolveOpenAPIPath())
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

func resolveOpenAPIPath() string {
	root, err := repoRoot()
	if err != nil {
		return openAPISourcePath
	}
	return filepath.Join(root, openAPISourcePath)
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
