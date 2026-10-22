package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// openAPISpecRelativePath is resolved from this package directory
// (internal/api) when `go test` runs.
const openAPISpecRelativePath = "../../docs/api/openapi.yaml"

// publicAdminOperations are intentionally reachable without admin auth.
// Keep this list minimal: every other /api/v1/admin path must be registered
// behind admin auth middleware AND documented with bearerAuth or cookieAuth.
var publicAdminOperations = map[string]bool{
	// Login must be public; it is rate limited and issues the admin session.
	"/api/v1/admin/login": true,
	// Logout only clears the auth cookie and is idempotent.
	"/api/v1/admin/logout": true,
}

// authWrappers are the middleware constructors that gate admin API routes.
// jwtAuth protects Bearer-token calls; the cookie wrappers protect same-origin
// EventSource routes that cannot send an Authorization header.
var authWrappers = map[string]bool{
	"jwtAuth":                   true,
	"adminCookieAuth":           true,
	"adminCookieAuthNoRedirect": true,
}

type openAPIDocument struct {
	Paths map[string]map[string]any `yaml:"paths"`
}

// TestAdminRoutesAreAuthenticatedAndDocumented guards two failure modes:
//
//  1. An /api/v1/admin route is registered without admin auth middleware.
//  2. The public OpenAPI document and the router drift apart (a documented
//     admin endpoint that does not exist, or a live endpoint missing from the
//     spec). Since /api/v1/openapi.{json,yaml} is public, drift is visible to
//     everyone and must fail CI.
func TestAdminRoutesAreAuthenticatedAndDocumented(t *testing.T) {
	routes := parseRouterAdminRoutes(t)
	documented := parseOpenAPIAdminOperations(t)

	// Router -> OpenAPI: every admin route must be documented and protected.
	for path, protected := range routes {
		if !publicAdminOperations[path] && !protected {
			t.Errorf("admin route %s is registered without jwtAuth/adminCookieAuth", path)
		}
		if _, ok := documented[path]; !ok {
			t.Errorf("admin route %s is registered but missing from %s", path, openAPISpecRelativePath)
		}
	}

	// OpenAPI -> router: every documented admin operation must exist and
	// declare an admin security scheme.
	for path, operations := range documented {
		if _, ok := routes[path]; !ok {
			t.Errorf("admin path %s is documented but not registered in package api", path)
		}
		if len(operations) == 0 {
			t.Errorf("admin path %s has no HTTP operations", path)
			continue
		}
		if publicAdminOperations[path] {
			continue
		}
		for method, operation := range operations {
			security := operationSecuritySchemes(operation)
			if !security["bearerAuth"] && !security["cookieAuth"] {
				t.Errorf("admin operation %s %s must declare bearerAuth or cookieAuth, got %v", method, path, security)
			}
		}
	}
}

func TestHandlerIsProtected(t *testing.T) {
	protected := map[string]bool{"protected": true}

	cases := []struct {
		name string
		expr string
		want bool
	}{
		{"direct wrapper", `jwtAuth(handler)`, true},
		{"cookie wrapper", `adminCookieAuthNoRedirect(handler)`, true},
		{"protected variable", `protected`, true},
		{"unprotected variable", `handler`, false},
		{"wrapper inside middleware chain", `csrfMiddleware(jwtAuth(handler))`, true},
		{"unprotected inside middleware chain", `csrfMiddleware(handler)`, false},
		{"handlerfunc keeps protection", `http.HandlerFunc(protected)`, true},
		{"all methods protected", `methodMux(map[string]http.Handler{http.MethodGet: protected, http.MethodPost: jwtAuth(handler)})`, true},
		{"one method unprotected", `methodMux(map[string]http.Handler{http.MethodGet: protected, http.MethodPost: handler})`, false},
		{"empty method map", `methodMux(map[string]http.Handler{})`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expr, err := parser.ParseExpr(tc.expr)
			if err != nil {
				t.Fatalf("parse expr: %v", err)
			}
			if got := handlerIsProtected(expr, protected); got != tc.want {
				t.Errorf("handlerIsProtected(%s) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestIsAdminAPIPath(t *testing.T) {
	for path, want := range map[string]bool{
		"/api/v1/admin":                true,
		"/api/v1/admin/":               true,
		"/api/v1/admin/users":          true,
		"/api/v1/administrators":       false,
		"/api/v1/events":               false,
		"/admin/users":                 false,
		"/api/v1/admin/scraper/events": true,
	} {
		if got := isAdminAPIPath(path); got != want {
			t.Errorf("isAdminAPIPath(%q) = %v, want %v", path, got, want)
		}
	}
}

// parseRouterAdminRoutes returns every /api/v1/admin pattern registered in the
// api package, mapped to whether its handler is wrapped in admin auth
// middleware. All non-test Go files in the package are scanned so a route
// registered outside router.go is still covered.
func parseRouterAdminRoutes(t *testing.T) map[string]bool {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("list package files: %v", err)
	}
	fset := token.NewFileSet()

	protectedVars := map[string]bool{}
	routes := map[string]bool{}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		// Assignments are visited in source order so a later reassignment can
		// revoke protection recorded by an earlier one.
		ast.Inspect(file, func(node ast.Node) bool {
			assign, ok := node.(*ast.AssignStmt)
			if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				return true
			}
			ident, ok := assign.Lhs[0].(*ast.Ident)
			if !ok {
				return true
			}
			protectedVars[ident.Name] = handlerIsProtected(assign.Rhs[0], protectedVars)
			return true
		})

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (selector.Sel.Name != "Handle" && selector.Sel.Name != "HandleFunc") {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			pattern, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			// Patterns may be "GET /api/v1/..." or just "/api/v1/...".
			if idx := strings.IndexByte(pattern, ' '); idx >= 0 {
				pattern = pattern[idx+1:]
			}
			if !isAdminAPIPath(pattern) {
				return true
			}
			// A path may be registered once per method (e.g. GET and PATCH on
			// /admin/scraper/config). It is only safe when every registration
			// is protected.
			protected := false
			// HandleFunc takes a plain func and therefore cannot be wrapped in
			// auth middleware; leave it unprotected so the test fails.
			if selector.Sel.Name == "Handle" {
				protected = handlerIsProtected(call.Args[1], protectedVars)
			}
			if existing, seen := routes[pattern]; seen {
				routes[pattern] = existing && protected
			} else {
				routes[pattern] = protected
			}
			return true
		})
	}

	if len(routes) == 0 {
		t.Fatal("no /api/v1/admin routes found in package api (parser regression?)")
	}
	return routes
}

// isAdminAPIPath reports whether path is the admin API root or below it.
func isAdminAPIPath(path string) bool {
	return path == "/api/v1/admin" || strings.HasPrefix(path, "/api/v1/admin/")
}

// isAuthWrapperCall reports whether expr is a call to one of authWrappers.
func isAuthWrapperCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	return ok && authWrappers[ident.Name]
}

// handlerIsProtected reports whether a handler expression is wrapped in admin
// auth. It understands direct wrappers, variables assigned from a wrapper,
// single-argument middleware chains (e.g. csrfMiddleware(jwtAuth(h))), and
// methodMux maps whose every method handler is protected.
func handlerIsProtected(expr ast.Expr, protectedVars map[string]bool) bool {
	switch typed := expr.(type) {
	case *ast.Ident:
		return protectedVars[typed.Name]

	case *ast.CallExpr:
		if isAuthWrapperCall(typed) {
			return true
		}
		ident, ok := typed.Fun.(*ast.Ident)
		if ok && ident.Name == "methodMux" {
			if len(typed.Args) != 1 {
				return false
			}
			composite, ok := typed.Args[0].(*ast.CompositeLit)
			if !ok || len(composite.Elts) == 0 {
				return false
			}
			for _, element := range composite.Elts {
				keyValue, ok := element.(*ast.KeyValueExpr)
				if !ok || !handlerIsProtected(keyValue.Value, protectedVars) {
					return false
				}
			}
			return true
		}
		// Unwrap middleware chains that take a single handler argument.
		if len(typed.Args) == 1 {
			return handlerIsProtected(typed.Args[0], protectedVars)
		}
	}
	return false
}

// parseOpenAPIAdminOperations returns every documented /api/v1/admin operation,
// keyed by path and then HTTP method.
func parseOpenAPIAdminOperations(t *testing.T) map[string]map[string]map[string]any {
	t.Helper()

	data, err := os.ReadFile(openAPISpecRelativePath)
	if err != nil {
		t.Fatalf("read %s: %v", openAPISpecRelativePath, err)
	}
	var doc openAPIDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", openAPISpecRelativePath, err)
	}

	operations := map[string]map[string]map[string]any{}
	for path, item := range doc.Paths {
		if !isAdminAPIPath(path) {
			continue
		}
		for method, raw := range item {
			operation, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			switch method {
			case "get", "post", "put", "patch", "delete", "head", "options":
				if operations[path] == nil {
					operations[path] = map[string]map[string]any{}
				}
				operations[path][strings.ToUpper(method)] = operation
			}
		}
	}

	if len(operations) == 0 {
		t.Fatal("no /api/v1/admin paths found in OpenAPI spec (parser regression?)")
	}
	return operations
}

// operationSecuritySchemes returns the security scheme names declared by an
// OpenAPI operation.
func operationSecuritySchemes(operation map[string]any) map[string]bool {
	schemes := map[string]bool{}
	raw, ok := operation["security"].([]any)
	if !ok {
		return schemes
	}
	for _, entry := range raw {
		requirement, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		for name := range requirement {
			schemes[name] = true
		}
	}
	return schemes
}
