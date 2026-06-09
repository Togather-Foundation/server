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

Optionally copy occurrences from the retired event and patch the canonical's
fields atomically in a single transaction.

Examples:
  server review merge evt_canonical evt_duplicate
  server review merge evt_canonical evt_duplicate --transfer-occurrences
  server review merge evt_canonical evt_duplicate --name "Better Name" --description "..."`,
		Args: cobra.ExactArgs(2),
		RunE: runReviewMerge,
	}

	reviewConsolidateCmd = &cobra.Command{
		Use:   "consolidate <canonical-id> <id2> [id3...]",
		Short: "Consolidate multiple duplicate events into a canonical event",
		Long: `Consolidate multiple events into one canonical event using the
events consolidate API. All non-canonical events are retired.

Optionally copy occurrences from retired events and patch the canonical's
fields atomically in a single transaction.

Examples:
  server review consolidate evt_canonical evt_dup1 evt_dup2 evt_dup3
  server review consolidate evt_canonical evt_dup1 evt_dup2 --transfer-occurrences
  server review consolidate evt_canonical evt_dup1 evt_dup2 evt_dup3 --name "Series Title"`,
		Args: cobra.MinimumNArgs(2),
		RunE: runReviewConsolidate,
	}

	mergeTransferOccurrences bool
	mergeName                string
	mergeDescription         string
	mergeImage               string
	mergeURL                 string
	mergeDomain              string
)

func init() {
	mergeFlags := []*cobra.Command{reviewMergeCmd, reviewConsolidateCmd}
	for _, cmd := range mergeFlags {
		cmd.Flags().BoolVar(&mergeTransferOccurrences, "transfer-occurrences", false, "Copy occurrences from retired events to canonical")
		cmd.Flags().StringVar(&mergeName, "name", "", "Patch canonical event name")
		cmd.Flags().StringVar(&mergeDescription, "description", "", "Patch canonical event description")
		cmd.Flags().StringVar(&mergeImage, "image", "", "Patch canonical event image URL")
		cmd.Flags().StringVar(&mergeURL, "url", "", "Patch canonical event public URL")
		cmd.Flags().StringVar(&mergeDomain, "domain", "", "Patch canonical event domain (arts, music, culture, sports, community, education, general)")
	}
}

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
	out := cmd.OutOrStdout()

	body := map[string]any{
		"event_ulid": canonicalID,
		"retire":     retireIDs,
	}

	if mergeTransferOccurrences {
		body["transfer_occurrences"] = true
	}

	if mergeName != "" || mergeDescription != "" || mergeImage != "" || mergeURL != "" || mergeDomain != "" {
		eventPatch := map[string]any{}
		if mergeName != "" {
			eventPatch["name"] = mergeName
		}
		if mergeDescription != "" {
			eventPatch["description"] = mergeDescription
		}
		if mergeImage != "" {
			eventPatch["image"] = mergeImage
		}
		if mergeURL != "" {
			eventPatch["url"] = mergeURL
		}
		if mergeDomain != "" {
			eventPatch["eventDomain"] = mergeDomain
		}
		body["event"] = eventPatch
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal consolidate body: %w", err)
	}

	u := fmt.Sprintf("%s/api/v1/admin/events/consolidate", serverURL)
	respBody, err := doPOST(client, u, bytes.NewReader(bodyBytes), jwt)
	if err != nil {
		return err
	}

	if reviewJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		var result map[string]any
		if err := json.Unmarshal(respBody, &result); err != nil {
			_, _ = fmt.Fprintln(out, string(respBody))
			return nil
		}
		return enc.Encode(result)
	}

	summary := fmt.Sprintf("✓ Consolidated %d event(s) into %s", len(retireIDs), canonicalID)
	if mergeTransferOccurrences {
		summary += " (occurrences transferred)"
	}
	_, _ = fmt.Fprintln(out, summary)
	return nil
}
