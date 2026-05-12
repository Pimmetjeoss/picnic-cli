// Hand-written novel commands: sql (SELECT-only escape hatch), since (time window over local store).

package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"picnic-pp-cli/internal/store"
)

func newSQLCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "sql <query>",
		Short:       "Run a read-only SQL query against the local Picnic store",
		Example:     "  picnic-pp-cli sql \"SELECT resource_type, COUNT(*) FROM resources GROUP BY resource_type\"",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			query := args[0]
			lower := strings.ToLower(strings.TrimSpace(query))
			if !strings.HasPrefix(lower, "select") && !strings.HasPrefix(lower, "with") && !strings.HasPrefix(lower, "pragma") {
				return fmt.Errorf("sql: only SELECT / WITH / PRAGMA statements allowed (got: %s)", firstWord(query))
			}
			for _, banned := range []string{"insert", "update", "delete", "drop", "alter", "create", "attach", "detach"} {
				if strings.Contains(lower, " "+banned+" ") || strings.HasPrefix(lower, banned+" ") {
					return fmt.Errorf("sql: %q is not allowed in read-only mode", banned)
				}
			}

			if dryRunOK(flags) {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{"would_run": query}, flags)
			}

			db, err := openStoreForCmd(flags)
			if err != nil {
				return err
			}
			defer db.Close()

			rows, err := db.Query(query)
			if err != nil {
				return fmt.Errorf("sql: %w", err)
			}
			defer rows.Close()

			cols, err := rows.Columns()
			if err != nil {
				return err
			}
			out := []map[string]any{}
			for rows.Next() {
				vals := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range vals {
					ptrs[i] = &vals[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					return err
				}
				row := map[string]any{}
				for i, c := range cols {
					row[c] = vals[i]
				}
				out = append(out, row)
			}
			if err := rows.Err(); err != nil {
				return err
			}
			return printJSONFiltered(cmd.OutOrStdout(), out, flags)
		},
	}
	return cmd
}

func newSinceCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "since <duration>",
		Short:       "Aggregate new deliveries, wallet entries, and articles seen since a relative time (e.g. 24h, 7d)",
		Example:     "  picnic-pp-cli since 7d --json\n  picnic-pp-cli since 24h",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			window, err := parseRelativeWindow(args[0])
			if err != nil {
				return err
			}
			cutoff := time.Now().Add(-window).UTC().Format(time.RFC3339)
			if dryRunOK(flags) {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"window": args[0],
					"cutoff": cutoff,
				}, flags)
			}

			db, err := openStoreForCmd(flags)
			if err != nil {
				return err
			}
			defer db.Close()

			counts := map[string]int{}
			for _, kind := range []string{"deliveries", "payment_wallet", "articles", "cart"} {
				row := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type=? AND synced_at >= ?`, kind, cutoff)
				var n int
				_ = row.Scan(&n)
				counts[kind] = n
			}
			return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
				"window": args[0],
				"cutoff": cutoff,
				"counts": counts,
			}, flags)
		},
	}
	return cmd
}

func parseRelativeWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	// Accept "Nd" as N*24h since time.ParseDuration doesn't.
	if strings.HasSuffix(s, "d") {
		num := strings.TrimSuffix(s, "d")
		var days int
		if _, err := fmt.Sscanf(num, "%d", &days); err != nil {
			return 0, fmt.Errorf("parse duration %q: %w", s, err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	if strings.HasSuffix(s, "w") {
		num := strings.TrimSuffix(s, "w")
		var weeks int
		if _, err := fmt.Sscanf(num, "%d", &weeks); err != nil {
			return 0, fmt.Errorf("parse duration %q: %w", s, err)
		}
		return time.Duration(weeks) * 7 * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", s, err)
	}
	return d, nil
}

func firstWord(s string) string {
	for i, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			return s[:i]
		}
	}
	return s
}

// openStoreForCmd is a thin wrapper used by the hand-written novel commands.
// It mirrors the lookup the generator uses for sync/search.
func openStoreForCmd(_ *rootFlags) (*store.Store, error) {
	return store.Open(defaultDBPath("picnic-pp-cli"))
}

// Decode a JSON-blob row's `data` column into a generic map.
func decodeResourceData(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}
