# Admin Template Partials

This directory contains reusable template partials for the SEL Admin interface.

## Available Partials

### `_head_meta.html`

Provides favicon links, Open Graph meta tags, Twitter Card tags, and general meta tags from togather.foundation branding.

**Usage:**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Your Page Title - SEL Admin</title>
    {{ template "_head_meta.html" . }}
    <link rel="stylesheet" href="/admin/static/css/tabler.min.css">
    <link rel="stylesheet" href="/admin/static/css/custom.css">
</head>
```

**Optional Context Variables:**

Pass these in your template data to customize the meta tags:

```go
data := map[string]interface{}{
    "Title":         "Dashboard - SEL Admin",
    "ActivePage":    "dashboard",
    
    // Optional meta tag overrides:
    "Description":   "Custom page description",
    "OGTitle":       "Custom Open Graph title",
    "OGDescription": "Custom OG description",
    "OGImage":       "/path/to/custom-image.jpg",
    "OGUrl":         "https://your-domain.com/page",
}
```

**Defaults:**

If not provided, the partial uses these defaults:
- **OG Title**: "SEL Admin - Togather Foundation"
- **OG Description**: "Community-owned infrastructure for event discovery. Shared Events Library admin interface."
- **OG Image**: `/admin/static/icons/togather_social_preview_v1.jpg`
- **OG URL**: `https://togather.foundation/`

### `_header.html`

Navigation header with user dropdown, logout, and theme toggle.

**Usage:**

```html
<body>
    {{ template "_header.html" . }}
    <!-- Your page content -->
</body>
```

**Required Context:**
- `ActivePage` (string): Current page identifier for navigation highlighting (e.g., "dashboard", "events", "users")

### `_footer.html`

Common footer with toast container, confirmation modal, and shared JavaScript includes.

**Usage:**

```html
    {{ template "_footer.html" . }}
    <script src="/admin/static/js/your-page-script.js"></script>
</body>
</html>
```

**Includes:**
- Toast notification container
- Confirmation modal dialog
- User modal partial (`_user_modal.html`)
- Core JavaScript: `tabler.min.js`, `api.js`, `components.js`

### `_user_modal.html`

Modal dialog for user management operations (create, edit, delete users).

**Note:** Automatically included by `_footer.html`, no need to include separately.

## Icon Assets

Favicon and branding assets are located in `/web/admin/static/icons/`:

- `favicon-16x16.png` - 16x16 favicon
- `favicon-32x32.png` - 32x32 favicon
- `apple-touch-icon.png` - 180x180 Apple touch icon
- `android-chrome-192x192.png` - 192x192 Android icon
- `android-chrome-512x512.png` - 512x512 Android icon
- `site.webmanifest` - Web app manifest
- `togather_social_preview_v1.jpg` - Social media preview image (1200x630)

All icon assets are sourced from https://togather.foundation.

## Naming Convention

Partial templates are prefixed with an underscore (`_`) to distinguish them from full page templates.

## Full Page Template Structure

Here's the recommended structure for a complete admin page:

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Page Title - SEL Admin</title>
    {{ template "_head_meta.html" . }}
    <link rel="stylesheet" href="/admin/static/css/tabler.min.css">
    <link rel="stylesheet" href="/admin/static/css/custom.css?v=3">
</head>
<body>
    {{ template "_header.html" . }}
    
    <!-- Page Content -->
    <div class="page">
        <div class="page-wrapper">
            <!-- Page header -->
            <div class="page-header d-print-none">
                <div class="container-xl">
                    <div class="row g-2 align-items-center">
                        <div class="col">
                            <h2 class="page-title">Your Page Title</h2>
                        </div>
                    </div>
                </div>
            </div>
            
            <!-- Page body -->
            <div class="page-body">
                <div class="container-xl">
                    <!-- Your content here -->
                </div>
            </div>
        </div>
    </div>
    
    {{ template "_footer.html" . }}
    <script src="/admin/static/js/your-page-script.js"></script>
</body>
</html>
```

## Template Data Contract

Every page handler should provide at minimum:

```go
data := map[string]interface{}{
    "Title":      "Page Title - SEL Admin",
    "ActivePage": "dashboard", // or "events", "users", "duplicates", "api-keys", "federation"
    // ... page-specific data
}
```
