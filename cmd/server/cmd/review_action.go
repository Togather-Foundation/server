package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

var (
	reviewApproveCmd = &cobra.Command{
		Use:   "approve <id>",
		Short: "Approve a review queue entry",
		Args:  cobra.ExactArgs(1),
		RunE:  runReviewApprove,
	}
	reviewRejectCmd = &cobra.Command{
		Use:   "reject <id>",
		Short: "Reject a review queue entry",
		Args:  cobra.ExactArgs(1),
		RunE:  runReviewReject,
	}
	reviewFixCmd = &cobra.Command{
		Use:   "fix <id>",
		Short: "Fix and approve a review queue entry",
		Args:  cobra.ExactArgs(1),
		RunE:  runReviewFix,
	}

	approveNotes        string
	approveRecordNotDup bool
	rejectReason        string
	rejectNotes         string
	fixNotes            string
	fixStartDate        string
	fixEndDate          string
)

func init() {
	reviewApproveCmd.Flags().StringVar(&approveNotes, "notes", "", "Optional review notes")
	reviewApproveCmd.Flags().BoolVar(&approveRecordNotDup, "record-not-duplicates", false, "Record duplicate warnings as not-duplicates")

	reviewRejectCmd.Flags().StringVar(&rejectReason, "reason", "", "Rejection reason (required)")
	reviewRejectCmd.Flags().StringVar(&rejectNotes, "notes", "", "Optional review notes")

	reviewFixCmd.Flags().StringVar(&fixNotes, "notes", "", "Optional review notes")
	reviewFixCmd.Flags().StringVar(&fixStartDate, "start-date", "", "Corrected start date (RFC3339)")
	reviewFixCmd.Flags().StringVar(&fixEndDate, "end-date", "", "Corrected end date (RFC3339)")
}

func runReviewApprove(cmd *cobra.Command, args []string) error {
	id, err := parseIntArg(args[0])
	if err != nil {
		return fmt.Errorf("invalid review ID: %w", err)
	}
	return reviewAction(cmd, id, "approve", map[string]any{
		"notes":                 approveNotes,
		"record_not_duplicates": approveRecordNotDup,
	})
}

func runReviewReject(cmd *cobra.Command, args []string) error {
	if rejectReason == "" {
		return fmt.Errorf("--reason is required for rejection")
	}
	id, err := parseIntArg(args[0])
	if err != nil {
		return fmt.Errorf("invalid review ID: %w", err)
	}
	return reviewAction(cmd, id, "reject", map[string]any{
		"reason": rejectReason,
		"notes":  rejectNotes,
	})
}

func runReviewFix(cmd *cobra.Command, args []string) error {
	id, err := parseIntArg(args[0])
	if err != nil {
		return fmt.Errorf("invalid review ID: %w", err)
	}

	body := map[string]any{
		"notes": fixNotes,
	}
	corrections := map[string]any{}
	if fixStartDate != "" {
		t, err := time.Parse(time.RFC3339, fixStartDate)
		if err != nil {
			return fmt.Errorf("invalid start-date: %w", err)
		}
		corrections["startDate"] = t
	}
	if fixEndDate != "" {
		t, err := time.Parse(time.RFC3339, fixEndDate)
		if err != nil {
			return fmt.Errorf("invalid end-date: %w", err)
		}
		corrections["endDate"] = t
	}
	if len(corrections) > 0 {
		body["corrections"] = corrections
	}

	return reviewAction(cmd, id, "fix", body)
}

func reviewAction(cmd *cobra.Command, id int, action string, bodyMap map[string]any) error {
	jwt, err := getReviewJWT()
	if err != nil {
		return err
	}

	serverURL := resolveReviewServerURL()

	var bodyBytes []byte
	if bodyMap != nil {
		bodyBytes, err = json.Marshal(bodyMap)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	u := fmt.Sprintf("%s/api/v1/admin/review-queue/%d/%s", serverURL, id, action)
	respBody, err := doPOST(client, u, bytes.NewReader(bodyBytes), jwt)
	if err != nil {
		return err
	}

	if reviewJSON {
		out := cmd.OutOrStdout()
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		var result map[string]any
		if err := json.Unmarshal(respBody, &result); err != nil {
			_, _ = fmt.Fprintln(out, string(respBody))
			return fmt.Errorf("unmarshal response: %w", err)
		}
		return enc.Encode(result)
	}

	actionLabel := action
	switch action {
	case "approve":
		actionLabel = "Approved"
	case "reject":
		actionLabel = "Rejected"
	case "fix":
		actionLabel = "Fixed"
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✓ %s review #%d\n", actionLabel, id)
	return nil
}

func parseIntArg(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid integer: %s", s)
	}
	return n, nil
}
