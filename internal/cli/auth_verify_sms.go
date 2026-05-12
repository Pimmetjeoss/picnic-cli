// Hand-written for Picnic: 2FA SMS verification step after auth login --send-sms.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"picnic-pp-cli/internal/config"
)

type picnicSMSVerifyRequest struct {
	OTP string `json:"otp"`
}

func newAuthVerifySMSCmd(flags *rootFlags) *cobra.Command {
	var email, password string
	cmd := &cobra.Command{
		Use:   "verify-sms <code>",
		Short: "Complete a 2FA SMS challenge initiated by 'auth login --send-sms'",
		Example: strings.Trim(`
  picnic-pp-cli auth verify-sms 123456
`, "\n"),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			code := args[0]
			if email == "" {
				email = firstNonEmpty(os.Getenv("PICNIC_USERNAME"), os.Getenv("LOGIN_NAME"))
			}
			if password == "" {
				password = firstNonEmpty(os.Getenv("PICNIC_PASSWORD"), os.Getenv("PASSWORD"))
			}
			if email == "" || password == "" {
				return fmt.Errorf("email/password required to complete the handshake (flags or env PICNIC_USERNAME/PICNIC_PASSWORD)")
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			baseURL := cfg.BaseURL
			if baseURL == "" {
				baseURL = "https://storefront-prod.nl.picnicinternational.com/api/15"
			}

			if flags.dryRun {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"would_post": baseURL + "/user/2fa/verify",
					"otp":        code,
				}, flags)
			}

			body := picnicSMSVerifyRequest{OTP: code}
			raw, _ := json.Marshal(body)
			req, err := http.NewRequestWithContext(cmd.Context(), "POST", baseURL+"/user/2fa/verify", bytes.NewReader(raw))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "picnic-pp-cli/0.1.0")
			req.Header.Set("x-picnic-agent", fmt.Sprintf("30100;%s;", picnicAppVersion))
			req.Header.Set("x-picnic-did", "picnic-pp-cli")

			httpClient := &http.Client{Timeout: 30 * time.Second}
			resp, err := httpClient.Do(req)
			if err != nil {
				return fmt.Errorf("2fa verify: %w", err)
			}
			defer resp.Body.Close()
			respBody, _ := io.ReadAll(resp.Body)

			if resp.StatusCode >= 400 {
				return fmt.Errorf("2fa verify failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
			}
			token := resp.Header.Get("x-picnic-auth")
			if token == "" {
				return fmt.Errorf("2fa verify accepted (HTTP %d) but x-picnic-auth header was empty; body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
			}
			cfg.AuthHeaderVal = ""
			if err := cfg.SaveCredential(token); err != nil {
				return configErr(err)
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"verified":    true,
					"config_path": cfg.Path,
				}, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "2FA verified. Token saved to %s\n", cfg.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "Picnic account email (env fallback)")
	cmd.Flags().StringVar(&password, "password", "", "Picnic account password (env fallback)")
	return cmd
}
