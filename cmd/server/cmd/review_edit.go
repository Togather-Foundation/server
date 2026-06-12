package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var (
	reviewEditCmd = &cobra.Command{
		Use:   "edit <review-id>",
		Short: "Edit event fields before approving",
		Long: `Update event fields (description, name, image, URL, domain) using the
review queue ID. Fetches the review entry to get the event ULID, then calls
PUT /api/v1/admin/events/{ulid}.

All fields are optional — only specified fields are updated.

Examples:
  server review edit 42 --description "Updated description"
  server review edit 42 --description "..." --name "New Name"
  server review edit 42 --description "..." --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: runReviewEdit,
	}

	editName        string
	editDescription string
	editImage       string
	editURL         string
	editDomain      string
	editDryRun      bool
)

func init() {
	reviewCmd.AddCommand(reviewEditCmd)
	reviewEditCmd.Flags().StringVar(&editName, "name", "", "Update event name")
	reviewEditCmd.Flags().StringVar(&editDescription, "description", "", "Update event description")
	reviewEditCmd.Flags().StringVar(&editImage, "image", "", "Update event image URL")
	reviewEditCmd.Flags().StringVar(&editURL, "url", "", "Update event public URL")
	reviewEditCmd.Flags().StringVar(&editDomain, "domain", "", "Update event domain (arts, music, culture, sports, community, education, general)")
	reviewEditCmd.Flags().BoolVar(&editDryRun, "dry-run", false, "Preview changes without executing")
}

func runReviewEdit(cmd *cobra.Command, args []string) error {
	id, err := parseIntArg(args[0])
	if err != nil {
		return fmt.Errorf("invalid review ID: %w", err)
	}

	if editName == "" && editDescription == "" && editImage == "" && editURL == "" && editDomain == "" {
		return fmt.Errorf("at least one of --name, --description, --image, --url, --domain is required")
	}

	jwt, err := getReviewJWT()
	if err != nil {
		return err
	}

	serverURL := resolveReviewServerURL()
	client := &http.Client{Timeout: 30 * time.Second}
	out := cmd.OutOrStdout()

	detail, err := fetchReviewDetail(client, serverURL, jwt, id)
	if err != nil {
		return fmt.Errorf("fetch review entry: %w", err)
	}

	body := map[string]any{}
	var patches []string
	if editName != "" {
		body["name"] = editName
		patches = append(patches, "name")
	}
	if editDescription != "" {
		body["description"] = editDescription
		patches = append(patches, "description")
	}
	if editImage != "" {
		body["image_url"] = editImage
		patches = append(patches, "image")
	}
	if editURL != "" {
		body["public_url"] = editURL
		patches = append(patches, "url")
	}
	if editDomain != "" {
		body["event_domain"] = editDomain
		patches = append(patches, "domain")
	}

	if editDryRun {
		if reviewJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{
				"dry_run":  true,
				"event_id": detail.EventID,
				"patches":  patches,
			})
		}
		_, _ = fmt.Fprintf(out, "Would update event %s (%s):\n", detail.EventID, detail.EventName)
		for _, p := range patches {
			_, _ = fmt.Fprintf(out, "  %s → \"%s\"\n", p, body[fieldKey(p)])
		}
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Run again without --dry-run to execute.")
		return nil
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal edit body: %w", err)
	}

	u := fmt.Sprintf("%s/api/v1/admin/events/%s", serverURL, detail.EventID)
	req, err := http.NewRequest(http.MethodPut, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create update request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("update event: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		rb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(rb))
	}

	if reviewJSON {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		var result map[string]any
		if err := json.Unmarshal(respBody, &result); err != nil {
			_, _ = fmt.Fprintln(out, string(respBody))
			return fmt.Errorf("unmarshal response: %w", err)
		}
		return enc.Encode(result)
	}

	_, _ = fmt.Fprintf(out, "✓ Updated event %s (%s): %s\n", detail.EventID, detail.EventName, joinPatches(patches))
	return nil
}

func fieldKey(p string) string {
	switch p {
	case "name":
		return "name"
	case "description":
		return "description"
	case "image":
		return "image_url"
	case "url":
		return "public_url"
	case "domain":
		return "event_domain"
	}
	return p
}

func joinPatches(patches []string) string {
	if len(patches) == 0 {
		return ""
	}
	if len(patches) == 1 {
		return patches[0]
	}
	out := ""
	for i, p := range patches {
		if i > 0 {
			if i == len(patches)-1 {
				out += " and "
			} else {
				out += ", "
			}
		}
		out += p
	}
	return out
}
