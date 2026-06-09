package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var (
	reviewMergeCmd = &cobra.Command{
		Use:   "merge <primary-id> <duplicate-id>",
		Short: "Merge a duplicate event into a primary event",
		Long: `Consolidate a duplicate event into a canonical primary event using
the events consolidate API. The duplicate event is retired.

Examples:
  server review merge evt_canonical evt_duplicate`,
		Args: cobra.ExactArgs(2),
		RunE: runReviewMerge,
	}

	reviewConsolidateCmd = &cobra.Command{
		Use:   "consolidate <canonical-id> <id2> [id3...]",
		Short: "Consolidate multiple duplicate events into a canonical event",
		Long: `Consolidate multiple events into one canonical event using the
events consolidate API. All non-canonical events are retired.

Examples:
  server review consolidate evt_canonical evt_dup1 evt_dup2 evt_dup3`,
		Args: cobra.MinimumNArgs(2),
		RunE: runReviewConsolidate,
	}
)

func runReviewMerge(cmd *cobra.Command, args []string) error {
	primaryID := args[0]
	duplicateID := args[1]

	jwt, err := getReviewJWT()
	if err != nil {
		return err
	}

	serverURL := resolveReviewServerURL()
	client := &http.Client{Timeout: 30 * time.Second}

	return consolidateEvents(client, serverURL, jwt, primaryID, []string{duplicateID}, cmd)
}

func runReviewConsolidate(cmd *cobra.Command, args []string) error {
	canonicalID := args[0]
	retireIDs := args[1:]

	jwt, err := getReviewJWT()
	if err != nil {
		return err
	}

	serverURL := resolveReviewServerURL()
	client := &http.Client{Timeout: 30 * time.Second}

	return consolidateEvents(client, serverURL, jwt, canonicalID, retireIDs, cmd)
}

func consolidateEvents(client *http.Client, serverURL, jwt, canonicalID string, retireIDs []string, cmd *cobra.Command) error {
	body, err := json.Marshal(map[string]any{
		"event_ulid": canonicalID,
		"retire":     retireIDs,
	})
	if err != nil {
		return fmt.Errorf("marshal consolidate body: %w", err)
	}

	u := fmt.Sprintf("%s/api/v1/admin/events/consolidate", serverURL)
	respBody, err := doPOST(client, u, bytes.NewReader(body), jwt)
	if err != nil {
		return err
	}

	if reviewJSON {
		out := cmd.OutOrStdout()
		var result map[string]any
		if err := json.Unmarshal(respBody, &result); err != nil {
			_, _ = fmt.Fprintln(out, string(respBody))
			return nil
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Printf("✓ Consolidated %d event(s) into %s\n", len(retireIDs), canonicalID)
	return nil
}
