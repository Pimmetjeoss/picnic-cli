// Hand-written: picnic-pp-cli recipes list / get — Picnic recipe browsing.
//
// Endpoints (reverse-engineered from live traffic May 2026):
//   - list:   GET /pages/meals-purchase-page-root  → ~25 purchased recipes for the user
//   - detail: GET /pages/selling-group-details-page?selling_group_id=<id>
//
// MRVDH's wrapper documented `/pages/recipe-details-page-root?recipe_id=X` but
// that path now 404s. Picnic renamed the surface to selling-group-details-page
// and `recipe_id` is now the `selling_group_id` carried in the OPEN action.

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

type recipeEntry struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
}

var recipeIDRegex = regexp.MustCompile(`"recipe_id"\s*:\s*"([^"]+)"`)

func newRecipesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipes",
		Short: "Browse your purchased Picnic recipes and fetch full recipe pages",
	}
	cmd.AddCommand(newRecipesListCmd(flags))
	cmd.AddCommand(newRecipesGetCmd(flags))
	return cmd
}

func newRecipesListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List your purchased recipes (their selling_group_id is the input to `recipes get`)",
		Example:     "  picnic-pp-cli recipes list --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			c := client.New(cfg, flags.timeout, flags.rateLimit)

			if dryRunOK(flags) {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"would_get": "/pages/meals-purchase-page-root",
				}, flags)
			}
			raw, err := c.Get("/pages/meals-purchase-page-root", nil)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			ids := extractRecipeIDs(raw)
			// Title-association is fragile on this page; ship ids and best-effort titles
			titles := extractRecipeTitles(raw, ids)
			out := make([]recipeEntry, 0, len(ids))
			for _, id := range ids {
				out = append(out, recipeEntry{ID: id, Title: titles[id]})
			}
			sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })

			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "%-30s  %s\n", "selling_group_id", "title")
			fmt.Fprintln(w, strings.Repeat("-", 80))
			for _, r := range out {
				fmt.Fprintf(w, "%-30s  %s\n", r.ID, r.Title)
			}
			fmt.Fprintf(w, "\n%d recipes (call `recipes get <id>` for details).\n", len(out))
			return nil
		},
	}
	return cmd
}

func newRecipesGetCmd(flags *rootFlags) *cobra.Command {
	var dayOffset int
	cmd := &cobra.Command{
		Use:     "get <selling_group_id>",
		Short:   "Get the full recipe detail page for a selling_group_id (from `recipes list`)",
		Example: "  picnic-pp-cli recipes get 61d2977b31f249268bf88da5 --json",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			id := args[0]
			cfg, err := config.Load(flags.configPath)
			if err != nil {
				return configErr(err)
			}
			c := client.New(cfg, flags.timeout, flags.rateLimit)

			path := "/pages/selling-group-details-page"
			params := map[string]string{"selling_group_id": id}
			if dayOffset > 0 {
				params["day_offset"] = fmt.Sprintf("%d", dayOffset)
			}

			if dryRunOK(flags) {
				return printJSONFiltered(cmd.OutOrStdout(), map[string]any{
					"would_get": path,
					"params":    params,
				}, flags)
			}
			raw, err := c.Get(path, params)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			// Return the raw Fusion page — it's large but agents can pluck what they need with --select.
			var pretty any
			_ = json.Unmarshal(raw, &pretty)
			return printJSONFiltered(cmd.OutOrStdout(), pretty, flags)
		},
	}
	cmd.Flags().IntVar(&dayOffset, "day-offset", 0, "Day offset (0 = today, 1 = tomorrow, ...)")
	return cmd
}

func extractRecipeIDs(raw []byte) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, m := range recipeIDRegex.FindAllSubmatch(raw, -1) {
		id := string(m[1])
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// extractRecipeTitles is best-effort: walks the JSON looking for "title" or
// "markdown" strings that appear close to each recipe_id. The Fusion page
// nests them deep inside PML stacks, so an exact pairing requires path
// matching; this implementation takes the first plausible title within a
// 4000-byte window after each recipe_id occurrence.
func extractRecipeTitles(raw []byte, ids []string) map[string]string {
	titleRx := regexp.MustCompile(`"(?:title|markdown)"\s*:\s*"([A-Za-zÀ-ÿ][^"]{2,80})"`)
	out := map[string]string{}
	for _, id := range ids {
		idQuoted := []byte(`"recipe_id":"` + id + `"`)
		idx := indexOf(raw, idQuoted)
		if idx < 0 {
			continue
		}
		// Search forward
		end := idx + 4000
		if end > len(raw) {
			end = len(raw)
		}
		for _, m := range titleRx.FindAllSubmatch(raw[idx:end], -1) {
			t := strings.TrimSpace(string(m[1]))
			if t == "" || strings.HasPrefix(t, "iglu:") || strings.HasPrefix(t, "tech.picnic") {
				continue
			}
			out[id] = t
			break
		}
	}
	return out
}

func indexOf(haystack, needle []byte) int {
	if len(needle) == 0 {
		return 0
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
