package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Togather-Foundation/server/internal/identity"
	"github.com/spf13/cobra"
)

// identityCmd mirrors the admin identity REST surface (srv-007 P1 T6):
// check | conflicts | link | reject. Auth follows the server review CLI
// (--key admin API key → STS exchange; --token pre-minted JWT; --server base URL).
var identityCmd = &cobra.Command{
	Use:   "identity",
	Short: "Inspect and adjudicate entity identity",
	Long: `Inspect and adjudicate entity identity for places and organizations.

Subcommands:
  check     <type> <ulid>            show one entity's identifiers, primary map, and decisions
  conflicts --type <type>            list identifier conflicts (unordered pairs sharing an identifier)
  link      <type> <ulid>            record an external identifier observation
  reject    <type> <ulid>            record that two entities are not duplicates

Auth (mirrors "server review"):
  --token JWT (skips STS exchange) → --key admin API key (STS exchange) → TOGATHER_ADMIN_API_KEY env

Examples:
  server identity check place 01ARZ3NDEKTSV4RRFFQ69G5FAV
  server identity conflicts --type place --limit 25
  server identity link place 01ARZ3NDEKTSV4RRFFQ69G5FAV --authority artsdata --uri https://kg.artsdata.ca/resource/K11-24
  server identity reject place 01ARZ3NDEKTSV4RRFFQ69G5FAV --other 01ARZ3NDEKTSV4RRFFQ69G5FAW --reason "different operator"
  server identity check place 01ARZ3NDEKTSV4RRFFQ69G5FAV --json`,
}

var (
	identityServerURL string
	identityAPIKey    string
	identityTokenFlag string
	identityJSON      bool
)

func init() {
	rootCmd.AddCommand(identityCmd)

	identityCmd.PersistentFlags().StringVar(&identityServerURL, "server", "", "Server base URL (env: TOGATHER_BASE_URL, default: http://localhost:8080)")
	identityCmd.PersistentFlags().StringVar(&identityAPIKey, "key", "", "Admin API key (env: TOGATHER_ADMIN_API_KEY)")
	identityCmd.PersistentFlags().StringVar(&identityTokenFlag, "token", "", "JWT token (skips STS exchange)")
	identityCmd.PersistentFlags().BoolVar(&identityJSON, "json", false, "JSON output")

	identityCmd.AddCommand(identityCheckCmd)
	identityCmd.AddCommand(identityConflictsCmd)
	identityCmd.AddCommand(identityLinkCmd)
	identityCmd.AddCommand(identityRejectCmd)
}

// --- auth resolution (mirrors server review) ------------------------------

func getIdentityJWT() (string, error) {
	if identityTokenFlag != "" {
		return identityTokenFlag, nil
	}

	serverURL := resolveIdentityServerURL()

	key := identityAPIKey
	if key == "" {
		key = os.Getenv("TOGATHER_ADMIN_API_KEY")
	}
	if key == "" {
		return "", fmt.Errorf("no API key provided; set --key, --token, or TOGATHER_ADMIN_API_KEY env")
	}

	return exchangeReviewJWT(serverURL, key)
}

func resolveIdentityServerURL() string {
	u := identityServerURL
	if u == "" {
		u = os.Getenv("TOGATHER_BASE_URL")
	}
	if u == "" {
		u = "http://localhost:8080"
	}
	if !strings.Contains(u, "://") && !strings.HasPrefix(u, "localhost") {
		u = "https://" + u
	}
	return u
}

func identityHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func isValidIdentityType(t string) bool {
	return t == string(identity.EntityTypePlace) || t == string(identity.EntityTypeOrganization)
}

// --- check ----------------------------------------------------------------

var identityCheckCmd = &cobra.Command{
	Use:   "check <type> <ulid>",
	Short: "Show one entity's identifiers, primary map, and decisions",
	Args:  cobra.ExactArgs(2),
	RunE:  runIdentityCheck,
}

func runIdentityCheck(cmd *cobra.Command, args []string) error {
	typ := args[0]
	if !isValidIdentityType(typ) {
		return fmt.Errorf("invalid entity type %q (must be place or organization)", typ)
	}
	ulid := args[1]

	jwt, err := getIdentityJWT()
	if err != nil {
		return err
	}

	serverURL := resolveIdentityServerURL()
	client := identityHTTPClient()
	out := cmd.OutOrStdout()

	u := fmt.Sprintf("%s/api/v1/admin/identity/%s/%s", serverURL, url.PathEscape(typ), url.PathEscape(ulid))
	body, err := doGET(client, u, jwt)
	if err != nil {
		return fmt.Errorf("fetch identity view: %w", err)
	}

	var view identity.IdentityView
	if err := json.Unmarshal(body, &view); err != nil {
		return fmt.Errorf("parse identity view: %w", err)
	}

	if identityJSON {
		return writeIndentedJSON(out, view)
	}

	printIdentityView(out, view)
	return nil
}

// --- conflicts ------------------------------------------------------------

var (
	identityConflictsCmd = &cobra.Command{
		Use:   "conflicts",
		Short: "List identifier conflicts",
		RunE:  runIdentityConflicts,
	}

	conflictsType              string
	conflictsLimit             int
	conflictsIncludeSuppressed bool
)

func init() {
	identityConflictsCmd.Flags().StringVar(&conflictsType, "type", "", "Entity type: place or organization (required)")
	identityConflictsCmd.Flags().IntVar(&conflictsLimit, "limit", 50, "Maximum conflicts to return")
	identityConflictsCmd.Flags().BoolVar(&conflictsIncludeSuppressed, "include-suppressed", false, "Include suppressed conflicts with their prior decision")
}

func runIdentityConflicts(cmd *cobra.Command, args []string) error {
	if !isValidIdentityType(conflictsType) {
		return fmt.Errorf("--type is required and must be place or organization")
	}

	jwt, err := getIdentityJWT()
	if err != nil {
		return err
	}

	serverURL := resolveIdentityServerURL()
	client := identityHTTPClient()
	out := cmd.OutOrStdout()

	params := url.Values{}
	params.Set("type", conflictsType)
	if conflictsLimit > 0 {
		params.Set("limit", fmt.Sprintf("%d", conflictsLimit))
	}
	if conflictsIncludeSuppressed {
		params.Set("include_suppressed", "true")
	}

	u := fmt.Sprintf("%s/api/v1/admin/identity/conflicts?%s", serverURL, params.Encode())
	body, err := doGET(client, u, jwt)
	if err != nil {
		return fmt.Errorf("fetch conflicts: %w", err)
	}

	var resp identity.ConflictsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse conflicts: %w", err)
	}

	if identityJSON {
		return writeIndentedJSON(out, resp)
	}

	printConflicts(out, conflictsType, resp)
	return nil
}

// --- link -----------------------------------------------------------------

var (
	identityLinkCmd = &cobra.Command{
		Use:   "link <type> <ulid>",
		Short: "Record an external identifier observation",
		Args:  cobra.ExactArgs(2),
		RunE:  runIdentityLink,
	}

	linkAuthority  string
	linkURI        string
	linkSource     string
	linkMethod     string
	linkConfidence float64
)

func init() {
	identityLinkCmd.Flags().StringVar(&linkAuthority, "authority", "", "Authority code (required, e.g. artsdata)")
	identityLinkCmd.Flags().StringVar(&linkURI, "uri", "", "External identifier URI (required)")
	identityLinkCmd.Flags().StringVar(&linkSource, "source", "manual", "Observation source (Phase 1 allow-list: manual)")
	identityLinkCmd.Flags().StringVar(&linkMethod, "method", "manual", "Observation method (Phase 1 allow-list: manual)")
	identityLinkCmd.Flags().Float64Var(&linkConfidence, "confidence", 1.0, "Confidence in [0,1]")
}

type identityLinkRequest struct {
	EntityType string  `json:"entity_type"`
	EntityID   string  `json:"entity_id"`
	Authority  string  `json:"authority"`
	URI        string  `json:"uri"`
	Method     string  `json:"method"`
	Confidence float64 `json:"confidence"`
	Source     string  `json:"source"`
}

func runIdentityLink(cmd *cobra.Command, args []string) error {
	typ := args[0]
	if !isValidIdentityType(typ) {
		return fmt.Errorf("invalid entity type %q (must be place or organization)", typ)
	}
	ulid := args[1]

	if strings.TrimSpace(linkAuthority) == "" {
		return fmt.Errorf("--authority is required")
	}
	if strings.TrimSpace(linkURI) == "" {
		return fmt.Errorf("--uri is required")
	}

	confidence := linkConfidence
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	body, err := json.Marshal(identityLinkRequest{
		EntityType: typ,
		EntityID:   ulid,
		Authority:  linkAuthority,
		URI:        linkURI,
		Method:     linkMethod,
		Confidence: confidence,
		Source:     linkSource,
	})
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	return postIdentityAction(cmd, "/api/v1/admin/identity/link", body, func(rec identity.DecisionRecord) string {
		return fmt.Sprintf("Linked %s %s to %s (%s)\n", rec.EntityType, rec.EntityID, linkURI, rec.ID)
	})
}

// --- reject ---------------------------------------------------------------

var (
	identityRejectCmd = &cobra.Command{
		Use:   "reject <type> <ulid>",
		Short: "Record that two entities are not duplicates",
		Args:  cobra.ExactArgs(2),
		RunE:  runIdentityReject,
	}

	identityRejectOther  string
	identityRejectReason string
)

func init() {
	identityRejectCmd.Flags().StringVar(&identityRejectOther, "other", "", "Counterpart entity ULID (required)")
	identityRejectCmd.Flags().StringVar(&identityRejectReason, "reason", "", "Reason the entities are distinct (required)")
}

type identityRejectRequest struct {
	EntityType    string `json:"entity_type"`
	EntityID      string `json:"entity_id"`
	CounterpartID string `json:"counterpart_id"`
	Reason        string `json:"reason"`
}

func runIdentityReject(cmd *cobra.Command, args []string) error {
	typ := args[0]
	if !isValidIdentityType(typ) {
		return fmt.Errorf("invalid entity type %q (must be place or organization)", typ)
	}
	ulid := args[1]

	if strings.TrimSpace(identityRejectOther) == "" {
		return fmt.Errorf("--other is required")
	}
	if strings.TrimSpace(identityRejectReason) == "" {
		return fmt.Errorf("--reason is required")
	}

	body, err := json.Marshal(identityRejectRequest{
		EntityType:    typ,
		EntityID:      ulid,
		CounterpartID: identityRejectOther,
		Reason:        identityRejectReason,
	})
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	return postIdentityAction(cmd, "/api/v1/admin/identity/reject", body, func(rec identity.DecisionRecord) string {
		return fmt.Sprintf("Rejected %s %s against %s (%s)\n", rec.EntityType, rec.EntityID, identityRejectOther, rec.ID)
	})
}

// postIdentityAction POSTs body to path and prints either a confirmation line
// or the decision record as JSON.
func postIdentityAction(cmd *cobra.Command, path string, body []byte, line func(identity.DecisionRecord) string) error {
	jwt, err := getIdentityJWT()
	if err != nil {
		return err
	}

	serverURL := resolveIdentityServerURL()
	client := identityHTTPClient()
	out := cmd.OutOrStdout()

	respBody, err := doPOST(client, serverURL+path, bytes.NewReader(body), jwt)
	if err != nil {
		return err
	}

	var rec identity.DecisionRecord
	if err := json.Unmarshal(respBody, &rec); err != nil {
		return fmt.Errorf("parse decision response: %w", err)
	}

	if identityJSON {
		return writeIndentedJSON(out, rec)
	}

	_, _ = fmt.Fprint(out, line(rec))
	return nil
}

func writeIndentedJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// --- table output ---------------------------------------------------------

func printIdentityView(out io.Writer, view identity.IdentityView) {
	_, _ = fmt.Fprintf(out, "Entity: %s %s\n", view.Ref.Type, view.Ref.ULID)

	if len(view.Primary) > 0 {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Primary:")
		for authority, uri := range view.Primary {
			_, _ = fmt.Fprintf(out, "  %s -> %s\n", authority, uri)
		}
	}

	if len(view.Identifiers) > 0 {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "Identifiers (%d):\n", len(view.Identifiers))
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "AUTHORITY\tURI\tMETHOD\tCONF\tPRIMARY\tSOURCE\tOBSERVED AT")
		for _, id := range view.Identifiers {
			primary := "no"
			if id.IsPrimary {
				primary = "yes"
			}
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%.2f\t%s\t%s\t%s\n",
				id.Authority, id.URI, id.Method, id.Confidence, primary, id.Source, formatObservedAt(id.ObservedAt))
		}
		_ = w.Flush()
	}

	if len(view.Decisions) > 0 {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "Decisions (%d):\n", len(view.Decisions))
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "ID\tACTION\tCREATED AT\tACTOR")
		for _, d := range view.Decisions {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				d.ID, d.Action, formatObservedAt(d.CreatedAt), d.Actor)
		}
		_ = w.Flush()
	}
}

func printConflicts(out io.Writer, typ string, resp identity.ConflictsResponse) {
	if len(resp.Items) == 0 {
		_, _ = fmt.Fprintf(out, "No %s conflicts found.\n", typ)
		return
	}

	_, _ = fmt.Fprintf(out, "Conflicts (%s) — %d:\n", typ, len(resp.Items))
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ENTITY\tCANDIDATE\tAUTHORITY\tURI\tSCORE\tSUPPRESSED")
	for _, item := range resp.Items {
		suppressed := "no"
		if item.Suppressed {
			suppressed = "yes"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%.2f\t%s\n",
			item.Ref.ULID, item.Candidate.ULID, item.Authority, item.URI, item.Score, suppressed)
	}
	_ = w.Flush()

	if resp.NextCursor != nil && *resp.NextCursor != "" {
		_, _ = fmt.Fprintf(out, "\n(next cursor: %s)\n", *resp.NextCursor)
	}
}

func formatObservedAt(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}
