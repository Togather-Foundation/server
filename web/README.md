# Web Files

This directory contains web files that are embedded into the server binary via `go:embed`.

## Auto-Generated Files

The following files are **automatically generated during Docker build** and should NOT be committed to git:

- `robots.txt` - SEO robots file with environment-specific sitemap URL
- `sitemap.xml` - Sitemap with environment-specific domain URLs

These files are generated using the `server webfiles` command during Docker build, with the domain determined by the deployment environment:

- **Production:** `togather.foundation`
- **Staging:** `staging.toronto.togather.foundation`  
- **Development:** `localhost:8080`

## Template Files

- `robots.txt.template` - Template/example for robots.txt
- `sitemap.xml.template` - Template/example for sitemap.xml

These templates are committed to git for reference and local development.

## Local Development

For local development without Docker:

```bash
# Generate webfiles for local testing
./server webfiles --domain localhost:8080 --output web/

# Build server with embedded files
make build
```

## Deployment

During deployment, the `deploy.sh` script passes the `DOMAIN` build arg to Docker based on the environment:

```bash
docker build \
  --build-arg DOMAIN=staging.toronto.togather.foundation \
  ...
```

The Dockerfile:
1. Builds the server binary
2. Runs `server webfiles --domain=$DOMAIN` to generate environment-specific files
3. Rebuilds the binary to embed the generated files via `go:embed`

## Why Not Commit Generated Files?

Generated files with environment-specific content should not be in version control because:

1. **Environment-specific:** Different domains per environment (prod/staging/dev)
2. **Build-time generation:** Generated during Docker build with correct domain
3. **Source of truth:** The templates and generation logic are the real source
4. **Merge conflicts:** Would cause conflicts across environment branches
5. **Best practice:** Auto-generated files belong in `.gitignore`

See: [Deployment Testing - Webfiles](../docs/deploy/webfiles-testing.md)
