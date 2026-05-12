// Hand-written Picnic-specific sync. The generic generator sync only handles
// GET-list endpoints; Picnic's rich data (deliveries, wallet) requires POST
// with array/object bodies. This command bypasses the generic engine and
// targets the actual endpoints the maintained MRVDH wrapper documents.

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"picnic-pp-cli/internal/client"
	"picnic-pp-cli/internal/config"
)

func newPicnicSyncCmd(flags *rootFlags) *cobra.Command {
	var walletPages int
	cmd := &cobra.Command{
		Use:   "picnic-sync",
		Short: "Picnic-specific sync: pulls cart, user, deliveries (incl. line items), wallet, delivery slots into the local store",
		Example: strings.Trim(`
  picnic-pp-cli picnic-sync
  picnic-pp-cli picnic-sync --wallet-pages 5
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"would_sync": []string{"user", "cart", "delivery_slots", "deliveries", "payment_wallet"},
				}, flags)
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			c := client.New(cfg, flags.timeout, flags.rateLimit)
			db, err := openStoreForCmd(flags)
			if err != nil {
				return err
			}
			defer db.Close()

			out := map[string]int{}
			w := cmd.ErrOrStderr()

			// 1. /user
			if raw, err := c.Get("/user", nil); err == nil {
				_ = db.Upsert("user", "self", raw)
				out["user"] = 1
			} else {
				fmt.Fprintf(w, "user: %v\n", err)
			}

			// 2. /cart
			if raw, err := c.Get("/cart", nil); err == nil {
				_ = db.Upsert("cart", "shopping_cart", raw)
				out["cart"] = 1
			} else {
				fmt.Fprintf(w, "cart: %v\n", err)
			}

			// 3. /cart/delivery_slots — keyed by slot_id
			if raw, err := c.Get("/cart/delivery_slots", nil); err == nil {
				var env struct {
					DeliverySlots []json.RawMessage `json:"delivery_slots"`
				}
				if json.Unmarshal(raw, &env) == nil {
					n := 0
					for _, s := range env.DeliverySlots {
						var m map[string]any
						_ = json.Unmarshal(s, &m)
						id, _ := m["slot_id"].(string)
						if id == "" {
							continue
						}
						if err := db.Upsert("delivery_slots", id, s); err == nil {
							n++
						}
					}
					out["delivery_slots"] = n
				}
			} else {
				fmt.Fprintf(w, "delivery_slots: %v\n", err)
			}

			// 4. POST /deliveries/summary [] → list of deliveries
			// Store each summary, then GET /deliveries/{id} for full line items.
			if raw, _, err := c.Post("/deliveries/summary", []any{}); err == nil {
				var summaries []map[string]any
				if json.Unmarshal(raw, &summaries) == nil {
					n := 0
					for _, s := range summaries {
						id, _ := s["delivery_id"].(string)
						if id == "" {
							continue
						}
						// Fetch full detail (carries line items) and store that.
						if full, err := c.Get("/deliveries/"+id, nil); err == nil {
							_ = db.Upsert("deliveries", id, full)
							n++
						} else {
							// Fallback: store the summary itself.
							b, _ := json.Marshal(s)
							_ = db.Upsert("deliveries", id, b)
							n++
						}
					}
					out["deliveries"] = n
				}
			} else {
				fmt.Fprintf(w, "deliveries: %v\n", err)
			}

			// 5. POST /wallet/transactions {page_number: N} — paginate
			pageTotal := 0
			for page := 1; page <= walletPages; page++ {
				raw, _, err := c.Post("/wallet/transactions", map[string]any{"page_number": page})
				if err != nil {
					fmt.Fprintf(w, "wallet page %d: %v\n", page, err)
					break
				}
				var txns []map[string]any
				if json.Unmarshal(raw, &txns) != nil || len(txns) == 0 {
					break
				}
				for _, t := range txns {
					id, _ := t["id"].(string)
					if id == "" {
						continue
					}
					b, _ := json.Marshal(t)
					if err := db.Upsert("payment_wallet", id, b); err == nil {
						pageTotal++
					}
				}
			}
			out["payment_wallet"] = pageTotal

			// 6. GET /pages/search-page-results?search_term=banaan — opportunistic article cache.
			// Not strictly necessary; left out of the default to keep sync fast and
			// avoid duplicating live-search results in the store.

			summary := map[string]any{
				"synced_at": time.Now().UTC().Format(time.RFC3339),
				"counts":    out,
			}
			return printJSONFiltered(cmd.OutOrStdout(), summary, flags)
		},
	}
	cmd.Flags().IntVar(&walletPages, "wallet-pages", 3, "How many wallet pages to fetch (1-based, default 3)")
	return cmd
}
