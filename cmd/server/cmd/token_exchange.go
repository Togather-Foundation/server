package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var tokenExchangeCmd = &cobra.Command{
	Use:   "token-exchange",
	Short: "Exchange an admin API key for a short-lived JWT via the STS endpoint",
	Long: `Exchanges an admin-role API key for a short-lived JWT by calling POST /api/v1/auth/token.

This is the canonical way to obtain admin JWTs for automation. It replaces
server admin-token for remote environments (admin-token remains for emergency
local use when the database is unavailable).

Examples:
  server token-exchange --key "$TOGATHER_ADMIN_API_KEY"
  server token-exchange --key "$TOGATHER_ADMIN_API_KEY" --server https://staging.togather.foundation
  server token-exchange --key "$TOGATHER_ADMIN_API_KEY" --json`,
	RunE: runTokenExchange,
}

var (
	tokenExchangeKey    string
	tokenExchangeServer string
	tokenExchangeJSON   bool
)

func init() {
	rootCmd.AddCommand(tokenExchangeCmd)
	tokenExchangeCmd.Flags().StringVar(&tokenExchangeKey, "key", "", "Admin API key (required, or set TOGATHER_ADMIN_API_KEY)")
	tokenExchangeCmd.Flags().StringVar(&tokenExchangeServer, "server", "", "Server base URL (default: http://localhost:8080)")
	tokenExchangeCmd.Flags().BoolVar(&tokenExchangeJSON, "json", false, "Output full JSON response")
}

func runTokenExchange(cmd *cobra.Command, args []string) error {
	if tokenExchangeKey == "" {
		tokenExchangeKey = os.Getenv("TOGATHER_ADMIN_API_KEY")
	}
	if tokenExchangeKey == "" {
		return fmt.Errorf("--key is required (or set TOGATHER_ADMIN_API_KEY)")
	}

	serverURL := tokenExchangeServer
	if serverURL == "" {
		serverURL = os.Getenv("TOGATHER_BASE_URL")
	}
	if serverURL == "" {
		serverURL = "http://localhost:8080"
	}

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/v1/auth/token", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tokenExchangeKey)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if tokenExchangeJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	fmt.Fprintln(cmd.OutOrStdout(), result.Token)
	return nil
}
