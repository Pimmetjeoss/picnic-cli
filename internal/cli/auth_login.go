// Hand-written for Picnic: md5-password handshake to obtain x-picnic-auth.
// Not generator-emitted; safe from regeneration.

package cli

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
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

const (
	picnicAppVersion  = "1.15.243-18832"
	picnicClientIDiOS = 20100
)

type picnicLoginRequest struct {
	Key      string `json:"key"`
	Secret   string `json:"secret"`
	ClientID int    `json:"client_id"`
}

type picnicLoginResponse struct {
	UserID         string `json:"user_id"`
	SecondFactor   string `json:"second_factor_type,omitempty"`
	ShowSecondAuth bool   `json:"show_second_auth,omitempty"`
	Error          *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	var (
		email      string
		password   string
		sendSMS    bool
		countryArg string
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to Picnic with email + password (md5 handshake) and store the x-picnic-auth token",
		Example: strings.Trim(`
  picnic-pp-cli auth login --email you@example.com --password 'hunter2'
  PICNIC_USERNAME=you@example.com PICNIC_PASSWORD=hunter2 picnic-pp-cli auth login
  picnic-pp-cli auth login --send-sms      # request a 2FA SMS code first
`, "\n"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if email == "" {
				email = firstNonEmpty(os.Getenv("PICNIC_USERNAME"), os.Getenv("LOGIN_NAME"))
			}
			if password == "" {
				password = firstNonEmpty(os.Getenv("PICNIC_PASSWORD"), os.Getenv("PASSWORD"))
			}
			if email == "" || password == "" {
				return fmt.Errorf("email and password required (flags --email/--password or env PICNIC_USERNAME/PICNIC_PASSWORD or LOGIN_NAME/PASSWORD)")
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			baseURL := cfg.BaseURL
			if countryArg != "" {
				baseURL = picnicCountryBaseURL(countryArg)
			}
			if baseURL == "" {
				baseURL = "https://storefront-prod.nl.picnicinternational.com/api/15"
			}

			if flags.dryRun {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"would_post": baseURL + "/user/login",
					"email":      email,
					"client_id":  picnicClientIDiOS,
					"send_sms":   sendSMS,
				}, flags)
			}

			body := picnicLoginRequest{
				Key:      email,
				Secret:   md5Hex(password),
				ClientID: picnicClientIDiOS,
			}
			raw, _ := json.Marshal(body)
			req, err := http.NewRequestWithContext(cmd.Context(), "POST", baseURL+"/user/login", bytes.NewReader(raw))
			if err != nil {
				return fmt.Errorf("build login request: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "picnic-pp-cli/0.1.0")
			req.Header.Set("x-picnic-agent", fmt.Sprintf("30100;%s;", picnicAppVersion))
			req.Header.Set("x-picnic-did", "picnic-pp-cli")
			// --send-sms is handled AFTER login (POST /user/2fa/generate). The
			// initial login call is the same; the 2FA generate call uses the
			// freshly-acquired (partial-access) token from the response.
			_ = sendSMS

			httpClient := &http.Client{Timeout: 30 * time.Second}
			resp, err := httpClient.Do(req)
			if err != nil {
				return fmt.Errorf("login request: %w", err)
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			var parsed picnicLoginResponse
			_ = json.Unmarshal(respBody, &parsed)

			if resp.StatusCode == 401 || resp.StatusCode == 403 {
				return fmt.Errorf("login failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
			}

			token := resp.Header.Get("x-picnic-auth")

			// 2FA required path: response carries no token, instead asks for SMS verification.
			if token == "" && (parsed.ShowSecondAuth || parsed.SecondFactor != "") {
				out := map[string]any{
					"two_factor_required": true,
					"second_factor":       parsed.SecondFactor,
					"next_step":           "Run: picnic-pp-cli auth verify-sms <code>",
					"base_url":            baseURL,
					"email":               email,
				}
				if flags.asJSON {
					return printJSONFiltered(cmd.OutOrStdout(), out, flags)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Picnic requires 2FA. A code was sent via", parsed.SecondFactor)
				fmt.Fprintln(cmd.OutOrStdout(), "Run: picnic-pp-cli auth verify-sms <code>")
				return nil
			}

			if token == "" {
				return fmt.Errorf("login succeeded (HTTP %d) but x-picnic-auth header was empty; body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
			}

			if countryArg != "" {
				cfg.BaseURL = baseURL
			}
			cfg.AuthHeaderVal = ""
			if err := cfg.SaveCredential(token); err != nil {
				return configErr(fmt.Errorf("saving token: %w", err))
			}

			// If --send-sms was set, fire the 2FA generate call now that we
			// have the partial-access token. Picnic returns HTTP 204 on success.
			smsSent := false
			if sendSMS {
				gReq, _ := http.NewRequestWithContext(cmd.Context(), "POST", baseURL+"/user/2fa/generate", bytes.NewReader([]byte(`{"channel":"SMS"}`)))
				gReq.Header.Set("Content-Type", "application/json")
				gReq.Header.Set("x-picnic-auth", token)
				gReq.Header.Set("x-picnic-agent", fmt.Sprintf("30100;%s;", picnicAppVersion))
				gReq.Header.Set("x-picnic-did", "picnic-pp-cli")
				gResp, gErr := httpClient.Do(gReq)
				if gErr == nil {
					gResp.Body.Close()
					smsSent = gResp.StatusCode == 204 || gResp.StatusCode == 200
				}
			}

			out := map[string]any{
				"logged_in":   true,
				"user_id":     parsed.UserID,
				"base_url":    baseURL,
				"config_path": cfg.Path,
				"acquired_at": time.Now().UTC().Format(time.RFC3339),
				"sms_sent":    smsSent,
			}
			if sendSMS && smsSent {
				out["next_step"] = "Run: picnic-pp-cli auth verify-sms <code>"
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in as %s. Token saved to %s\n", email, cfg.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "Picnic account email (or env PICNIC_USERNAME / LOGIN_NAME)")
	cmd.Flags().StringVar(&password, "password", "", "Picnic account password (or env PICNIC_PASSWORD / PASSWORD)")
	cmd.Flags().BoolVar(&sendSMS, "send-sms", false, "Request a 2FA SMS code (use auth verify-sms <code> afterwards)")
	cmd.Flags().StringVar(&countryArg, "country", "", "Override country (nl, de, fr); rewrites base URL")
	return cmd
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

func picnicCountryBaseURL(c string) string {
	c = strings.ToLower(strings.TrimSpace(c))
	if c == "" {
		c = "nl"
	}
	return fmt.Sprintf("https://storefront-prod.%s.picnicinternational.com/api/15", c)
}
