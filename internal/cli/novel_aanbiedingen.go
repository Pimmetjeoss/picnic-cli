// Hand-written: picnic-pp-cli aanbiedingen — list this week's promotions.
// Picnic exposes them on the /pages/promo-page-root Fusion page; the tile
// objects (SELLING_UNIT_TILE) carry product info, and the surrounding PML
// components carry the human-readable promo labels (1+1 gratis, 10% korting,
// 2 voor €X, nu €X). We walk the page tree, collect tiles, then attach the
// nearest promotion_label / accessibilityLabel that lives in the same subtree.

package cli

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"picnic-pp-cli/internal/client"
	"picnic-pp-cli/internal/config"
)

type promoItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Unit       string `json:"unit_quantity,omitempty"`
	PriceCents int    `json:"price_cents,omitempty"`
	PriceEUR   string `json:"price_eur,omitempty"`
	Promo      string `json:"promo,omitempty"`
}

var promoRegex = regexp.MustCompile(`(?i)(\d+\s*\+\s*\d+\s*gratis|\d+\s*%\s*korting|\d+\s+voor\s+€\s*\d+|\bnu\s+€\s*\d+(?:[.,]\d{1,2})?|\bvan\s+€\s*\d+(?:[.,]\d{1,2})?|\bgratis\b|tweede\s+gratis|\bvoordeelpr|aanbieding)`)

func newAanbiedingenCmd(flags *rootFlags) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "aanbiedingen",
		Short: "List this week's Picnic promotions (live from /pages/promo-page-root)",
		Example: strings.Trim(`
  picnic-pp-cli aanbiedingen --json
  picnic-pp-cli aanbiedingen --limit 20
`, "\n"),
		Aliases: []string{"promotions", "deals"},
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			c := client.New(cfg, flags.timeout, flags.rateLimit)

			if dryRunOK(flags) {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"would_get": "/pages/promo-page-root",
					"limit":     limit,
				}, flags)
			}

			raw, err := c.Get("/pages/promo-page-root", nil)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			var page any
			if err := json.Unmarshal(raw, &page); err != nil {
				return fmt.Errorf("parse promo page: %w", err)
			}

			items := extractPromoItems(page)
			// Sort: items WITH a promo label first, then by name.
			sort.SliceStable(items, func(i, j int) bool {
				if (items[i].Promo == "") != (items[j].Promo == "") {
					return items[i].Promo != "" // promo-tagged first
				}
				return items[i].Name < items[j].Name
			})
			if limit > 0 && len(items) > limit {
				items = items[:limit]
			}

			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				return printJSONFiltered(cmd.OutOrStdout(), items, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-12s  %-14s  %-9s  %-26s  %s\n", "id", "unit", "price", "promo", "product")
			fmt.Fprintln(w, strings.Repeat("-", 120))
			for _, it := range items {
				promo := it.Promo
				if len(promo) > 26 {
					promo = promo[:23] + "..."
				}
				fmt.Fprintf(w, "%-12s  %-14s  %-9s  %-26s  %s\n", it.ID, truncShortAanb(it.Unit, 14), it.PriceEUR, promo, it.Name)
			}
			fmt.Fprintf(w, "\n%d products on /pages/promo-page-root\n", len(items))
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "Cap the number of items returned (0 = all)")
	return cmd
}

// extractPromoItems walks the Fusion page and returns one entry per
// SELLING_UNIT_TILE, with the nearest promotion_label / accessibilityLabel /
// markdown string in the same subtree treated as the promo description.
func extractPromoItems(page any) []promoItem {
	items := map[string]*promoItem{}

	// We pass the ENCLOSING tile dict into the recursion so the scoped scan
	// only sees promo-strings within that tile's subtree.
	type tileFound struct {
		id     string
		name   string
		unit   string
		price  int
		subtree any
	}

	var findTiles func(o any, out *[]tileFound)
	findTiles = func(o any, out *[]tileFound) {
		switch x := o.(type) {
		case map[string]any:
			if su, ok := x["sellingUnit"].(map[string]any); ok {
				id, _ := su["id"].(string)
				if strings.HasPrefix(id, "s") {
					name, _ := su["name"].(string)
					unit, _ := su["unit_quantity"].(string)
					if unit == "" {
						unit, _ = su["unitQuantity"].(string)
					}
					price := 0
					switch p := su["display_price"].(type) {
					case float64:
						price = int(p)
					case int:
						price = p
					}
					*out = append(*out, tileFound{id: id, name: name, unit: unit, price: price, subtree: x})
				}
			}
			for _, v := range x {
				findTiles(v, out)
			}
		case []any:
			for _, v := range x {
				findTiles(v, out)
			}
		}
	}

	var tiles []tileFound
	findTiles(page, &tiles)

	var scanPromo func(o any, hits *[]string)
	scanPromo = func(o any, hits *[]string) {
		switch x := o.(type) {
		case map[string]any:
			for k, v := range x {
				if s, ok := v.(string); ok {
					switch k {
					case "promotion_label", "accessibilityLabel", "markdown":
						if promoRegex.MatchString(s) {
							*hits = append(*hits, strings.TrimSpace(s))
						}
					}
				}
				scanPromo(v, hits)
			}
		case []any:
			for _, v := range x {
				scanPromo(v, hits)
			}
		}
	}

	for _, t := range tiles {
		if _, seen := items[t.id]; seen {
			continue
		}
		var hits []string
		scanPromo(t.subtree, &hits)
		promo := ""
		for _, h := range hits {
			if h == "" || h == t.name {
				continue
			}
			// Prefer the shortest informative label; "1+1 gratis" beats the
			// full accessibility sentence "Cup-a-soup & Good Noodles 1+1 gratis".
			short := promoRegex.FindString(h)
			if short != "" && (promo == "" || len(short) < len(promo)) {
				promo = strings.TrimSpace(short)
			}
		}
		price := ""
		if t.price > 0 {
			price = fmt.Sprintf("€%.2f", float64(t.price)/100.0)
		}
		items[t.id] = &promoItem{
			ID: t.id, Name: t.name, Unit: t.unit,
			PriceCents: t.price, PriceEUR: price, Promo: promo,
		}
	}

	out := make([]promoItem, 0, len(items))
	for _, v := range items {
		out = append(out, *v)
	}
	return out
}

func truncShortAanb(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
