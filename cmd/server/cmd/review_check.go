package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

var reviewCheckCmd = &cobra.Command{
	Use:   "check <review-id>",
	Short: "Deep inspect a review queue entry",
	Args:  cobra.ExactArgs(1),
	RunE:  runReviewCheck,
}

func runReviewCheck(cmd *cobra.Command, args []string) error {
	id, err := parseIntArg(args[0])
	if err != nil {
		return fmt.Errorf("invalid review ID: %w", err)
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
		return err
	}

	if reviewJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(detail)
	}

	printReviewDetail(out, detail)
	return nil
}

func printReviewDetail(out interface{ Write([]byte) (int, error) }, detail *ReviewQueueDetail) {
	_, _ = fmt.Fprintf(out, "Review #%d — %s\n", detail.ID, detail.Status)
	_, _ = fmt.Fprintf(out, "  Event: %s (%s)\n", detail.EventName, detail.EventID)

	startDate := "-"
	if detail.EventStartTime != nil {
		startDate = detail.EventStartTime.Format("2006-01-02")
	}
	_, _ = fmt.Fprintf(out, "  Date: %s\n", startDate)
	_, _ = fmt.Fprintf(out, "  Age: %s\n", formatAge(detail.CreatedAt))

	if detail.SourceID != nil {
		_, _ = fmt.Fprintf(out, "  Source: %s\n", *detail.SourceID)
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "Warnings (%d):\n", len(detail.Warnings))
	for _, w := range detail.Warnings {
		_, _ = fmt.Fprintf(out, "  [%s] %s: %s\n", w.Code, w.Field, w.Message)
	}

	if len(detail.Changes) > 0 {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Changes:")
		for _, c := range detail.Changes {
			_, _ = fmt.Fprintf(out, "  %s: %v → %v (%s)\n", c.Field, c.Original, c.Corrected, c.Reason)
		}
	}

	if len(detail.RelatedEvents) > 0 {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Related Events:")
		for _, re := range detail.RelatedEvents {
			_, _ = fmt.Fprintf(out, "  %s — %s", re.ULID, re.Name)
			if re.Similarity != nil {
				_, _ = fmt.Fprintf(out, " (%.2f)", *re.Similarity)
			}
			_, _ = fmt.Fprintln(out)
		}
	}

	if detail.ReviewNotes != nil && *detail.ReviewNotes != "" {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "Notes: %s\n", *detail.ReviewNotes)
	}

	if detail.RejectionReason != nil && *detail.RejectionReason != "" {
		_, _ = fmt.Fprintf(out, "Rejection reason: %s\n", *detail.RejectionReason)
	}
}
