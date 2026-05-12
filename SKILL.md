---
name: pp-picnic
description: "Every Picnic wrapper feature, plus a local SQLite store, offline FTS, and reorder/spend-trend/slot-watch that no... Trigger phrases: `boodschappen op picnic bestellen`, `wanneer komt mijn picnic bezorging`, `wat heb ik bij picnic uitgegeven`, `bestel deze picnic levering opnieuw`, `use picnic`, `run picnic`."
author: "Pimmetjeoss"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - picnic-pp-cli
    install:
      - kind: go
        bins: [picnic-pp-cli]
        module: github.com/mvanhorn/printing-press-library/library/shopping/picnic/cmd/picnic-pp-cli
---

# Picnic — Printing Press CLI

## Prerequisites: Install the CLI

This skill drives the `picnic-pp-cli` binary. **You must verify the CLI is installed before invoking any command from this skill.** If it is missing, install it first:

1. Install via the Printing Press installer:
   ```bash
   npx -y @mvanhorn/printing-press install picnic --cli-only
   ```
2. Verify: `picnic-pp-cli --version`
3. Ensure `$GOPATH/bin` (or `$HOME/go/bin`) is on `$PATH`.

If the `npx` install fails before this CLI has a public-library category, install Node or use the category-specific Go fallback after publish.

If `--version` reports "command not found" after install, the install step did not put the binary on `$PATH`. Do not proceed with skill commands until verification succeeds.


picnic-pp-cli wraps the same API every community wrapper hits, then layers a local SQLite store on top so commands like `reorder`, `spend-trend`, and `pantry-recurring` can answer 'do this again' and 'where is my money going' without burning round trips. Pair `--json` with `--select` on any command and it slots straight into an agent loop.

## When to Use This CLI

Use picnic-pp-cli when an agent (or you on the command line) needs to drive Picnic groceries — search, build a cart, reserve slots, track deliveries, or analyse spend — and wants typed JSON output instead of a mobile UI. It is also the only Picnic tool with a local store, so it is the right choice when the agent needs to answer historical questions like 'what did I buy last March' without hitting the API.

## Unique Capabilities

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`search`** — Search every article you've ever seen on Picnic locally, instantly, with no API hit.

  _Use this when an agent needs grocery lookups offline or in a tight loop; saves token spend and API round-trips._

  ```bash
  picnic-pp-cli search 'havermelk' --json --select id,name,price
  ```
- **`reorder`** — Re-add every article from a previous delivery to your current cart in one shot.

  _Use this when the user wants to repeat last week's groceries; one command instead of 30 add-to-cart calls._

  ```bash
  picnic-pp-cli reorder dlv_42 --dry-run --json
  ```
- **`spend-trend`** — See spend per month or category from your wallet history, with deltas vs prior period.

  _Use this for budget questions: 'am I spending more on groceries this month than last?'_

  ```bash
  picnic-pp-cli spend-trend --by month --since 2026-01-01 --json
  ```
- **`pantry-recurring`** — Predict re-buy candidates: articles ordered N or more times in the last K weeks.

  _Use this to seed a weekly cart automatically from past behaviour._

  ```bash
  picnic-pp-cli pantry-recurring --weeks 12 --min 3 --json
  ```
- **`drift`** — Surface price changes on watched articles across local store snapshots.

  _Use this when you want to know when staples get cheaper or sneak up in price._

  ```bash
  picnic-pp-cli drift --watch favorites.json --json
  ```

### Agent-native plumbing
- **`slot-watch`** — Poll for delivery slots that match a weekday/time-window filter and exit zero when one opens.

  _Use this when peak slots are sold out and you want a cron-style alert when one opens._

  ```bash
  picnic-pp-cli slot-watch --weekday wed,thu --before 18:00 --interval 5m
  ```
- **`since`** — Aggregate new deliveries, wallet entries, and cart edits since a relative timestamp.

  _Use this at the start of a session to catch up an agent on what happened in the user's grocery world._

  ```bash
  picnic-pp-cli since 7d --json
  ```
- **`sql`** — Read-only SQL against the local Picnic store for ad-hoc questions.

  _Use this to answer questions the typed commands don't cover._

  ```bash
  picnic-pp-cli sql 'SELECT name, MAX(price_cents)/100.0 FROM articles ORDER BY 2 DESC LIMIT 10'
  ```
- **`export`** — Dump the entire local store as JSON, CSV, or NDJSON.

  _Use this when the user wants their Picnic history in a spreadsheet or a downstream pipeline._

  ```bash
  picnic-pp-cli export --format ndjson > picnic.ndjson
  ```
- **`doctor`** — Health check: auth valid, base URL reachable, store schema OK.

  _Use this first when something looks off; tells you whether to fix auth, network, or store._

  ```bash
  picnic-pp-cli doctor --json
  ```

## Command Reference

**articles** — Picnic product catalogue: search, suggest, and inspect articles

- `picnic-pp-cli articles category` — Get the category an article belongs to
- `picnic-pp-cli articles find` — Search Picnic articles by keyword (live API)
- `picnic-pp-cli articles get` — Get full detail for a single article (product)
- `picnic-pp-cli articles suggest` — Get product suggestions for a partial query

**cart** — Active shopping cart for the authenticated user

- `picnic-pp-cli cart add` — Add an article to the cart
- `picnic-pp-cli cart checkout` — Confirm and place the order for the current cart
- `picnic-pp-cli cart clear` — Remove all items from the cart
- `picnic-pp-cli cart get` — Get the current cart with items, totals, and selected delivery slot
- `picnic-pp-cli cart remove` — Remove an article from the cart
- `picnic-pp-cli cart set_slot` — Reserve a delivery slot for the current cart

**categories** — Browse Picnic's product category hierarchy

- `picnic-pp-cli categories` — Get the product category tree with configurable depth

**deliveries** — Past and active deliveries

- `picnic-pp-cli deliveries cancel` — Cancel a scheduled delivery (only before cutoff)
- `picnic-pp-cli deliveries get` — Get a specific delivery
- `picnic-pp-cli deliveries list` — List deliveries; pass --summary for the lightweight payload
- `picnic-pp-cli deliveries position` — Get the live driver position for an in-flight delivery
- `picnic-pp-cli deliveries rate` — Submit a 0-10 rating for a completed delivery
- `picnic-pp-cli deliveries resend_invoice` — Resend the delivery invoice email
- `picnic-pp-cli deliveries scenario` — Get delivery route/scenario detail (driver, ETA window)

**delivery_slots** — Available delivery time windows

- `picnic-pp-cli delivery_slots` — List delivery slots currently bookable from this cart

**lists** — Shopping lists (saved product collections)

- `picnic-pp-cli lists get` — Get a specific shopping list with its items
- `picnic-pp-cli lists list` — List all shopping lists

**mgm** — Member-get-member (referral) program

- `picnic-pp-cli mgm` — Get referral program details and your invite link

**payment** — Payment profile and wallet

- `picnic-pp-cli payment profile` — Get the user's payment profile and methods
- `picnic-pp-cli payment wallet` — Paginated wallet transaction history
- `picnic-pp-cli payment wallet_detail` — Get full breakdown of a single wallet transaction

**user** — Authenticated Picnic user profile and account details

- `picnic-pp-cli user get` — Get the authenticated user's profile, address, and household
- `picnic-pp-cli user get-settings` — Get user settings and feature flags for the authenticated user


**Hand-written commands**

- `picnic-pp-cli auth login` — Log in with email + password; performs the md5-password handshake and stores the x-picnic-auth token in config
- `picnic-pp-cli auth status` — Show whether a valid auth token is configured and where it came from
- `picnic-pp-cli auth logout` — Forget the stored x-picnic-auth token
- `picnic-pp-cli auth verify-sms <code>` — Complete an SMS 2FA challenge issued during login
- `picnic-pp-cli sync` — Sync deliveries, wallet, cart, lists, categories, and seen articles into the local SQLite store
- `picnic-pp-cli sql <query>` — Run a read-only SQL query against the local Picnic store
- `picnic-pp-cli export` — Export the local store as JSON, CSV, or NDJSON
- `picnic-pp-cli slot-watch` — Watch delivery slots and alert when one matching --window / --weekday / --before becomes available
- `picnic-pp-cli reorder <delivery-id>` — Re-add every article from a past delivery to the current cart (with --dry-run to preview)
- `picnic-pp-cli spend-trend` — Spend per month / week, broken down by category, computed from synced wallet transactions
- `picnic-pp-cli pantry-recurring` — Predict re-buy candidates from delivery history (articles ordered N+ times across last K weeks)
- `picnic-pp-cli drift` — Show price changes on watched articles across local store snapshots
- `picnic-pp-cli since <duration>` — What changed in Picnic for me since <duration> (e.g. since 7d): new deliveries, wallet, cart edits
- `picnic-pp-cli doctor` — Health check: auth valid, base URL reachable, store schema OK
- `picnic-pp-cli country [nl|de|fr]` — Show or change the Picnic country (alters the base URL)


### Finding the right command

When you know what you want to do but not which command does it, ask the CLI directly:

```bash
picnic-pp-cli which "<capability in your own words>"
```

`which` resolves a natural-language capability query to the best matching command from this CLI's curated feature index. Exit code `0` means at least one match; exit code `2` means no confident match — fall back to `--help` or use a narrower query.

## Recipes


### Reorder last week's groceries

```bash
picnic-pp-cli deliveries list --active --json --select id,delivery_time.start | jq -r '.[0].id' | xargs picnic-pp-cli reorder
```

Pipe the most recent delivery id into `reorder` to re-add every line item to the current cart.

### Find the cheapest oat milk you've ever bought

```bash
picnic-pp-cli sql "SELECT name, MIN(price_cents)/100.0 AS min_eur FROM articles WHERE name LIKE '%havermelk%' GROUP BY name ORDER BY min_eur ASC LIMIT 5"
```

SQL escape hatch over the local article snapshots.

### Watch for a Thursday evening slot

```bash
picnic-pp-cli slot-watch --weekday thu --before 19:00 --interval 5m && say 'Slot open'
```

slot-watch exits zero only when a matching slot appears; chain whatever notifier you like.

### Bound a list response with --select

```bash
picnic-pp-cli deliveries list --json --select id,delivery_time.start,total_price.amount
```

Pair `--json` with dotted `--select` on Picnic's nested responses to avoid burning agent context on the full delivery payload.

### Predict next week's groceries

```bash
picnic-pp-cli pantry-recurring --weeks 12 --min 3 --json --select article_id,name,count
```

Articles bought 3+ times in 12 weeks — seed for a weekly cart.

## Auth Setup

Picnic has no API keys. `picnic-pp-cli auth login --email <your-email> --password '...'` MD5-hashes your password, POSTs `/user/login`, and stores the `x-picnic-auth` token in `~/.config/picnic-pp-cli/config.toml`. If Picnic challenges with 2FA SMS, run `picnic-pp-cli auth verify-sms <code>` to finish the handshake. After that every command picks up the stored token automatically.

Run `picnic-pp-cli doctor` to verify setup.

## Agent Mode

Add `--agent` to any command. Expands to: `--json --compact --no-input --no-color --yes`.

- **Pipeable** — JSON on stdout, errors on stderr
- **Filterable** — `--select` keeps a subset of fields. Dotted paths descend into nested structures; arrays traverse element-wise. Critical for keeping context small on verbose APIs:

  ```bash
  picnic-pp-cli articles get mock-value --agent --select id,name,status
  ```
- **Previewable** — `--dry-run` shows the request without sending
- **Offline-friendly** — sync/search commands can use the local SQLite store when available
- **Non-interactive** — never prompts, every input is a flag
- **Explicit retries** — use `--idempotent` only when an already-existing create should count as success

### Mutation safety (cart + delivery commands)

`cart checkout` places a real order — money leaves the user's account, groceries are
scheduled for delivery. This is the one command an agent MUST always preview with
`--dry-run` and then get explicit human confirmation before running for real.

The other cart mutations (`cart add`, `cart remove`, `cart clear`, `cart set-slot`) and
`reorder <id> --confirm` are reversible from the Picnic app or via the inverse CLI
command. By default agents should still preview these with `--dry-run` and confirm
once. If the user has said something like "skip the preview" or "snel" for the
current session, agents may run these mutations directly and report the result
afterwards — but only those four. `cart checkout` is the no-shortcut command.

| Command | Default behavior | Shortcut allowed? |
|---------|------------------|-------------------|
| `cart add` / `remove` / `clear` / `set-slot` | dry-run → confirm → execute | yes, on explicit user request |
| `reorder <id>` | dry-run prints plan; `--confirm` required to mutate | yes, on explicit user request |
| `deliveries cancel <id>` | preview → `--confirm` flag required | yes, on explicit user request |
| `cart checkout` | dry-run → confirm → execute | **no — always preview + confirm** |
| Read commands (`cart get`, `deliveries list`, etc.) | run directly | always |

### Response envelope

Commands that read from the local store or the API wrap output in a provenance envelope:

```json
{
  "meta": {"source": "live" | "local", "synced_at": "...", "reason": "..."},
  "results": <data>
}
```

Parse `.results` for data and `.meta.source` to know whether it's live or local. A human-readable `N results (live)` summary is printed to stderr only when stdout is a terminal — piped/agent consumers get pure JSON on stdout.

## Agent Feedback

When you (or the agent) notice something off about this CLI, record it:

```
picnic-pp-cli feedback "the --since flag is inclusive but docs say exclusive"
picnic-pp-cli feedback --stdin < notes.txt
picnic-pp-cli feedback list --json --limit 10
```

Entries are stored locally at `~/.picnic-pp-cli/feedback.jsonl`. They are never POSTed unless `PICNIC_FEEDBACK_ENDPOINT` is set AND either `--send` is passed or `PICNIC_FEEDBACK_AUTO_SEND=true`. Default behavior is local-only.

Write what *surprised* you, not a bug report. Short, specific, one line: that is the part that compounds.

## Output Delivery

Every command accepts `--deliver <sink>`. The output goes to the named sink in addition to (or instead of) stdout, so agents can route command results without hand-piping. Three sinks are supported:

| Sink | Effect |
|------|--------|
| `stdout` | Default; write to stdout only |
| `file:<path>` | Atomically write output to `<path>` (tmp + rename) |
| `webhook:<url>` | POST the output body to the URL (`application/json` or `application/x-ndjson` when `--compact`) |

Unknown schemes are refused with a structured error naming the supported set. Webhook failures return non-zero and log the URL + HTTP status on stderr.

## Named Profiles

A profile is a saved set of flag values, reused across invocations. Use it when a scheduled agent calls the same command every run with the same configuration - HeyGen's "Beacon" pattern.

```
picnic-pp-cli profile save briefing --json
picnic-pp-cli --profile briefing articles get mock-value
picnic-pp-cli profile list --json
picnic-pp-cli profile show briefing
picnic-pp-cli profile delete briefing --yes
```

Explicit flags always win over profile values; profile values win over defaults. `agent-context` lists all available profiles under `available_profiles` so introspecting agents discover them at runtime.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 2 | Usage error (wrong arguments) |
| 3 | Resource not found |
| 4 | Authentication required |
| 5 | API error (upstream issue) |
| 7 | Rate limited (wait and retry) |
| 10 | Config error |

## Argument Parsing

Parse `$ARGUMENTS`:

1. **Empty, `help`, or `--help`** → show `picnic-pp-cli --help` output
2. **Starts with `install`** → ends with `mcp` → MCP installation; otherwise → see Prerequisites above
3. **Anything else** → Direct Use (execute as CLI command with `--agent`)

## MCP Server Installation

1. Install the MCP server:
   ```bash
   go install github.com/mvanhorn/printing-press-library/library/shopping/picnic/cmd/picnic-pp-mcp@latest
   ```
2. Register with Claude Code:
   ```bash
   claude mcp add picnic-pp-mcp -- picnic-pp-mcp
   ```
3. Verify: `claude mcp list`

## Direct Use

1. Check if installed: `which picnic-pp-cli`
   If not found, offer to install (see Prerequisites at the top of this skill).
2. Match the user query to the best command from the Unique Capabilities and Command Reference above.
3. Execute with the `--agent` flag:
   ```bash
   picnic-pp-cli <command> [subcommand] [args] --agent
   ```
4. If ambiguous, drill into subcommand help: `picnic-pp-cli <command> --help`.
