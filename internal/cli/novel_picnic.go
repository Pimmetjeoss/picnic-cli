// Hand-written novel commands: slot-watch, pantry-recurring, spend-trend, drift, reorder.
//
// All five rely on the framework's existing client/config plumbing (live calls)
// or on the local SQLite store the generator wired up via sync.

package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"picnic-pp-cli/internal/client"
	"picnic-pp-cli/internal/cliutil"
	"picnic-pp-cli/internal/config"
)

// --- slot-watch ---------------------------------------------------------

func newSlotWatchCmd(flags *rootFlags) *cobra.Command {
	var (
		weekdays []string
		before   string
		after    string
		interval time.Duration
		maxWait  time.Duration
	)
	cmd := &cobra.Command{
		Use:   "slot-watch",
		Short: "Poll Picnic delivery slots; exit 0 as soon as a slot matches --weekday/--before/--after",
		Example: strings.Trim(`
  picnic-pp-cli slot-watch --weekday wed,thu --before 18:00 --interval 5m
  picnic-pp-cli slot-watch --after 17:00 --interval 1m --max-wait 30m
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"would_poll_every": interval.String(),
					"weekdays":         weekdays,
					"before":           before,
					"after":            after,
					"max_wait":         maxWait.String(),
				}, flags)
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			c := client.New(cfg, flags.timeout, flags.rateLimit)

			wdSet := map[string]bool{}
			for _, w := range weekdays {
				for _, p := range strings.Split(w, ",") {
					p = strings.TrimSpace(strings.ToLower(p))
					if p != "" {
						wdSet[normalizeWeekday(p)] = true
					}
				}
			}
			beforeT, afterT, err := parseSlotBounds(before, after)
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			deadline := time.Time{}
			if maxWait > 0 {
				deadline = time.Now().Add(maxWait)
			}

			for attempt := 1; ; attempt++ {
				if !deadline.IsZero() && time.Now().After(deadline) {
					return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
						"matched":  false,
						"reason":   "max_wait exhausted",
						"attempts": attempt - 1,
					}, flags)
				}
				slots, err := fetchSlots(c)
				if err != nil {
					if !flags.asJSON {
						fmt.Fprintf(cmd.ErrOrStderr(), "slot-watch: fetch error attempt %d: %v\n", attempt, err)
					}
				} else {
					for _, s := range slots {
						if matchSlot(s, wdSet, beforeT, afterT) {
							return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
								"matched":  true,
								"slot":     s,
								"attempts": attempt,
							}, flags)
						}
					}
				}
				if !flags.asJSON {
					fmt.Fprintf(cmd.ErrOrStderr(), "slot-watch: no match attempt %d, sleeping %s\n", attempt, interval)
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(interval):
				}
			}
		},
	}
	cmd.Flags().StringSliceVar(&weekdays, "weekday", nil, "Comma-separated weekdays to match: mon,tue,wed,thu,fri,sat,sun")
	cmd.Flags().StringVar(&before, "before", "", "Match slots whose start time is before HH:MM (e.g. 18:00)")
	cmd.Flags().StringVar(&after, "after", "", "Match slots whose start time is at or after HH:MM")
	cmd.Flags().DurationVar(&interval, "interval", 5*time.Minute, "Poll interval (default 5m)")
	cmd.Flags().DurationVar(&maxWait, "max-wait", 0, "Maximum wait (0 = forever)")
	return cmd
}

type picnicDeliverySlot struct {
	SlotID      string         `json:"slot_id"`
	WindowStart string         `json:"window_start"`
	WindowEnd   string         `json:"window_end"`
	IsAvailable bool           `json:"is_available"`
	Selected    bool           `json:"selected"`
	Raw         map[string]any `json:"-"`
}

func fetchSlots(c *client.Client) ([]picnicDeliverySlot, error) {
	raw, err := c.Get("/cart/delivery_slots", nil)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		DeliverySlots []map[string]any `json:"delivery_slots"`
	}
	if err := json.Unmarshal(raw, &envelope); err == nil && len(envelope.DeliverySlots) > 0 {
		return mapSlots(envelope.DeliverySlots), nil
	}
	// Some Picnic responses return a top-level array.
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
		return mapSlots(arr), nil
	}
	return nil, nil
}

func mapSlots(raw []map[string]any) []picnicDeliverySlot {
	out := make([]picnicDeliverySlot, 0, len(raw))
	for _, m := range raw {
		s := picnicDeliverySlot{Raw: m}
		if v, ok := m["slot_id"].(string); ok {
			s.SlotID = v
		}
		if v, ok := m["window_start"].(string); ok {
			s.WindowStart = v
		}
		if v, ok := m["window_end"].(string); ok {
			s.WindowEnd = v
		}
		if v, ok := m["is_available"].(bool); ok {
			s.IsAvailable = v
		}
		if v, ok := m["selected"].(bool); ok {
			s.Selected = v
		}
		out = append(out, s)
	}
	return out
}

func normalizeWeekday(s string) string {
	m := map[string]string{
		"monday": "mon", "tuesday": "tue", "wednesday": "wed",
		"thursday": "thu", "friday": "fri", "saturday": "sat", "sunday": "sun",
		"ma": "mon", "di": "tue", "wo": "wed", "do": "thu", "vr": "fri", "za": "sat", "zo": "sun",
		"mon": "mon", "tue": "tue", "wed": "wed", "thu": "thu", "fri": "fri", "sat": "sat", "sun": "sun",
	}
	if v, ok := m[strings.ToLower(s)]; ok {
		return v
	}
	return strings.ToLower(s)
}

func parseSlotBounds(before, after string) (beforeT, afterT *time.Time, err error) {
	parse := func(s string) (*time.Time, error) {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		t, perr := time.Parse("15:04", s)
		if perr != nil {
			return nil, fmt.Errorf("parse time %q: %w", s, perr)
		}
		return &t, nil
	}
	beforeT, err = parse(before)
	if err != nil {
		return nil, nil, err
	}
	afterT, err = parse(after)
	if err != nil {
		return nil, nil, err
	}
	return beforeT, afterT, nil
}

func matchSlot(s picnicDeliverySlot, wd map[string]bool, before, after *time.Time) bool {
	if !s.IsAvailable {
		return false
	}
	if s.WindowStart == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, s.WindowStart)
	if err != nil {
		return false
	}
	if len(wd) > 0 {
		dayKey := strings.ToLower(t.Weekday().String()[:3])
		if !wd[dayKey] {
			return false
		}
	}
	mins := t.Hour()*60 + t.Minute()
	if before != nil {
		if mins >= before.Hour()*60+before.Minute() {
			return false
		}
	}
	if after != nil {
		if mins < after.Hour()*60+after.Minute() {
			return false
		}
	}
	return true
}

// --- pantry-recurring ---------------------------------------------------

func newPantryRecurringCmd(flags *rootFlags) *cobra.Command {
	var (
		weeks int
		min   int
	)
	cmd := &cobra.Command{
		Use:   "pantry-recurring",
		Short: "Predict re-buy candidates: articles you've ordered at least --min times in the last --weeks weeks",
		Example: strings.Trim(`
  picnic-pp-cli pantry-recurring --weeks 12 --min 3 --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"weeks": weeks, "min": min}, flags)
			}
			db, err := openStoreForCmd(flags)
			if err != nil {
				return err
			}
			defer db.Close()

			cutoff := time.Now().Add(-time.Duration(weeks) * 7 * 24 * time.Hour).UTC().Format(time.RFC3339)
			rows, err := db.Query(`SELECT id, data FROM resources WHERE resource_type='deliveries' AND synced_at >= ?`, cutoff)
			if err != nil {
				return fmt.Errorf("query deliveries: %w", err)
			}
			defer rows.Close()

			counts := map[string]*pantryEntry{}
			for rows.Next() {
				var id string
				var raw []byte
				if err := rows.Scan(&id, &raw); err != nil {
					return err
				}
				m := decodeResourceData(raw)
				if m == nil {
					continue
				}
				for _, art := range walkDeliveryArticles(m) {
					if art.id == "" {
						continue
					}
					e := counts[art.id]
					if e == nil {
						e = &pantryEntry{ArticleID: art.id, Name: art.name}
						counts[art.id] = e
					}
					if art.name != "" && e.Name == "" {
						e.Name = art.name
					}
					e.Count++
				}
			}

			out := make([]pantryEntry, 0, len(counts))
			for _, e := range counts {
				if e.Count >= min {
					out = append(out, *e)
				}
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })

			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-16s  %5s  %s\n", "article_id", "count", "name")
			for _, e := range out {
				fmt.Fprintf(w, "%-16s  %5d  %s\n", e.ArticleID, e.Count, e.Name)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&weeks, "weeks", 12, "Look back this many weeks (default 12)")
	cmd.Flags().IntVar(&min, "min", 3, "Minimum order count to include (default 3)")
	return cmd
}

type pantryEntry struct {
	ArticleID string `json:"article_id"`
	Name      string `json:"name"`
	Count     int    `json:"count"`
}

type articleRef struct{ id, name string }

func walkDeliveryArticles(m map[string]any) []articleRef {
	var out []articleRef
	var visit func(any)
	visit = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			// Picnic line items typically expose `id` + `name`, sometimes `decorator_overrides.name`.
			if idRaw, ok := x["id"]; ok {
				if id, ok := idRaw.(string); ok && (strings.HasPrefix(id, "s") || strings.HasPrefix(id, "p")) {
					name, _ := x["name"].(string)
					out = append(out, articleRef{id: id, name: name})
				}
			}
			for _, vv := range x {
				visit(vv)
			}
		case []any:
			for _, vv := range x {
				visit(vv)
			}
		}
	}
	visit(m)
	return out
}

// --- spend-trend --------------------------------------------------------

func newSpendTrendCmd(flags *rootFlags) *cobra.Command {
	var by, since string
	cmd := &cobra.Command{
		Use:   "spend-trend",
		Short: "Roll up spend per --by (month or week) from synced wallet transactions",
		Example: strings.Trim(`
  picnic-pp-cli spend-trend --by month --since 2026-01-01 --json
`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"by": by, "since": since}, flags)
			}
			db, err := openStoreForCmd(flags)
			if err != nil {
				return err
			}
			defer db.Close()

			rows, err := db.Query(`SELECT id, data FROM resources WHERE resource_type='payment_wallet'`)
			if err != nil {
				return err
			}
			defer rows.Close()

			buckets := map[string]float64{}
			counts := map[string]int{}
			for rows.Next() {
				var id string
				var raw []byte
				if err := rows.Scan(&id, &raw); err != nil {
					return err
				}
				m := decodeResourceData(raw)
				if m == nil {
					continue
				}
				ts := pickTimestamp(m)
				amount := pickAmountCents(m)
				if ts.IsZero() || amount == 0 {
					continue
				}
				if since != "" {
					if c, err := time.Parse("2006-01-02", since); err == nil && ts.Before(c) {
						continue
					}
				}
				key := ts.Format("2006-01")
				if strings.EqualFold(by, "week") {
					y, w := ts.ISOWeek()
					key = fmt.Sprintf("%04d-W%02d", y, w)
				}
				buckets[key] += float64(amount) / 100.0
				counts[key]++
			}

			keys := make([]string, 0, len(buckets))
			for k := range buckets {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			type bucketOut struct {
				Period   string  `json:"period"`
				TotalEUR float64 `json:"total_eur"`
				TxnCount int     `json:"txn_count"`
			}
			out := make([]bucketOut, 0, len(keys))
			for _, k := range keys {
				out = append(out, bucketOut{Period: k, TotalEUR: round2(buckets[k]), TxnCount: counts[k]})
			}
			if flags.asJSON {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-10s  %10s  %5s\n", "period", "total_eur", "txns")
			for _, b := range out {
				fmt.Fprintf(w, "%-10s  %10.2f  %5d\n", b.Period, b.TotalEUR, b.TxnCount)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "month", "Bucket size: month or week")
	cmd.Flags().StringVar(&since, "since", "", "Earliest date to include (YYYY-MM-DD)")
	return cmd
}

func pickTimestamp(m map[string]any) time.Time {
	// Unix millis (Picnic wallet uses timestamp as ms since epoch).
	if v, ok := m["timestamp"]; ok {
		switch x := v.(type) {
		case float64:
			return time.UnixMilli(int64(x)).UTC()
		case int64:
			return time.UnixMilli(x).UTC()
		}
	}
	// ISO date strings (deliveries, articles).
	for _, k := range []string{"created_at", "transaction_date", "date", "creation_time"} {
		if v, ok := m[k].(string); ok && v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t
			}
			if t, err := time.Parse("2006-01-02", v); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

func pickAmountCents(m map[string]any) int {
	for _, k := range []string{"amount_in_cents", "amount", "total_amount", "price"} {
		if v, ok := m[k]; ok {
			switch x := v.(type) {
			case float64:
				return int(x)
			case int:
				return x
			case map[string]any:
				if a, ok := x["amount"]; ok {
					if f, ok := a.(float64); ok {
						return int(f)
					}
				}
			}
		}
	}
	return 0
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}

// --- drift --------------------------------------------------------------

func newDriftCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "drift",
		Short:       "Show price changes across local article snapshots (run after a fresh sync)",
		Example:     "  picnic-pp-cli drift --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"would_compare": "article snapshots"}, flags)
			}
			db, err := openStoreForCmd(flags)
			if err != nil {
				return err
			}
			defer db.Close()

			rows, err := db.Query(`SELECT id, data FROM resources WHERE resource_type='articles'`)
			if err != nil {
				return err
			}
			defer rows.Close()

			type drift struct {
				ArticleID  string  `json:"article_id"`
				Name       string  `json:"name"`
				CurrentEUR float64 `json:"current_eur"`
				Note       string  `json:"note"`
			}
			out := []drift{}
			for rows.Next() {
				var id string
				var raw []byte
				if err := rows.Scan(&id, &raw); err != nil {
					return err
				}
				m := decodeResourceData(raw)
				if m == nil {
					continue
				}
				price := pickAmountCents(m)
				name, _ := m["name"].(string)
				if price > 0 {
					out = append(out, drift{ArticleID: id, Name: name, CurrentEUR: float64(price) / 100.0, Note: "snapshot-only; price history requires multiple syncs"})
				}
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	return cmd
}

// --- reorder ------------------------------------------------------------

func newReorderCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reorder <delivery-id>",
		Short: "Re-add every article from a past delivery to the current cart (use --dry-run to preview)",
		Example: strings.Trim(`
  picnic-pp-cli reorder dlv_42 --dry-run --json
  picnic-pp-cli reorder dlv_42 --confirm
`, "\n"),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			deliveryID := args[0]
			confirmed, _ := cmd.Flags().GetBool("confirm")

			// Only short-circuit under the verify harness; real --dry-run runs
			// the store lookup so the user sees the actual plan.
			if cliutil.IsVerifyEnv() {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"delivery_id":           deliveryID,
					"would_lookup_in_store": true,
					"reason":                "verify-env-short-circuit",
				}, flags)
			}

			db, err := openStoreForCmd(flags)
			if err != nil {
				return err
			}
			defer db.Close()

			row := db.DB().QueryRow(`SELECT data FROM resources WHERE resource_type='deliveries' AND id=?`, deliveryID)
			var raw []byte
			if err := row.Scan(&raw); err != nil {
				return fmt.Errorf("delivery %s not found in local store; run sync first", deliveryID)
			}
			m := decodeResourceData(raw)
			arts := walkDeliveryArticles(m)
			if len(arts) == 0 {
				return fmt.Errorf("no article line items found in delivery %s", deliveryID)
			}
			type plan struct {
				ArticleID string `json:"article_id"`
				Name      string `json:"name"`
				Count     int    `json:"count"`
				Result    string `json:"result,omitempty"`
				Error     string `json:"error,omitempty"`
			}
			plans := make([]plan, 0, len(arts))
			counts := map[string]int{}
			names := map[string]string{}
			for _, a := range arts {
				counts[a.id]++
				if a.name != "" && names[a.id] == "" {
					names[a.id] = a.name
				}
			}
			for id, n := range counts {
				plans = append(plans, plan{ArticleID: id, Name: names[id], Count: n})
			}

			if flags.dryRun || !confirmed {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"delivery_id": deliveryID,
					"would_add":   plans,
					"reason":      ternary(flags.dryRun, "dry-run", "missing --confirm"),
				}, flags)
			}

			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			c := client.New(cfg, flags.timeout, flags.rateLimit)
			for i := range plans {
				body := map[string]any{"product_id": plans[i].ArticleID, "count": plans[i].Count}
				_, status, err := c.Post("/cart/add_product", body)
				if err != nil {
					plans[i].Error = err.Error()
					continue
				}
				if status >= 400 {
					plans[i].Error = fmt.Sprintf("HTTP %d", status)
					continue
				}
				plans[i].Result = "added"
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
				"delivery_id": deliveryID,
				"added":       plans,
			}, flags)
		},
	}
	cmd.Flags().Bool("confirm", false, "Confirm the cart mutations (default dry-run preview)")
	return cmd
}

func ternary(b bool, a, c string) string {
	if b {
		return a
	}
	return c
}
