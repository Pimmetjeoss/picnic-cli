# Picnic CLI

**Every Picnic wrapper feature, plus a local SQLite store, offline FTS, and reorder/spend-trend/slot-watch that no Picnic tool ships.**

picnic-pp-cli wraps the same API every community wrapper hits, then layers a local SQLite store on top so commands like `reorder`, `spend-trend`, and `pantry-recurring` can answer 'do this again' and 'where is my money going' without burning round trips. Pair `--json` with `--select` on any command and it slots straight into an agent loop.

Learn more at [Picnic](https://picnic.app).

## What this CLI can do

### Auth & onboarding
- `auth login` — email + password (md5 handshake)
- `auth login --send-sms` — when Picnic enforces 2FA, fires the SMS challenge automatically
- `auth verify-sms <code>` — completes the 2FA challenge
- `auth status` / `auth set-token` / `auth logout` — token management
- `doctor` — health check (config, auth, API reachability, local store)

### Cart
- `cart get` — what's currently in your cart
- `cart add --product-id <id> --count N` — add an article (`--dry-run` previews the request)
- `cart remove --product-id <id> --count N` — remove
- `cart clear` — empty the cart
- `cart set-slot --slot-id <id>` — reserve a delivery slot
- `cart checkout` — place the order

### Product discovery
- `articles find --term <q>` — live Fusion-page search against Picnic NL/DE/FR
- `articles suggest --term <q>` — auto-complete suggestions
- `articles get <id>` — full product details page

### Deliveries
- `delivery-slots` — every bookable slot
- `deliveries list` — your delivery history
- `deliveries get <id>` — full delivery detail (line items, totals)
- `deliveries position <id>` — live driver location + ETA
- `deliveries scenario <id>` — route and projected arrival
- `deliveries cancel <id> --confirm` — cancel a scheduled delivery
- `deliveries rate <id> --score 0-10` — submit a delivery rating
- `deliveries resend-invoice <id>` — re-send the invoice email

### Payments & profile
- `user get` / `user get-settings` — profile, feature flags, redacted phone
- `payment profile` — stored payment methods
- `payment wallet --page N` — paginated transaction history
- `payment wallet-detail <id>` — full per-transaction breakdown (delivery lines, etc.)
- `mgm` — member-get-member referral details

### Novel commands (only this CLI ships them)
- `picnic-sync` — pulls cart, user, slots, full deliveries (with line items), and paginated wallet into a local SQLite store
- `reorder <delivery-id> --dry-run` — shows exactly what would be re-added to the cart from a past delivery; `--confirm` actually mutates
- `pantry-recurring --weeks 12 --min 3` — articles you've ordered N+ times in the last K weeks
- `spend-trend --by month` — monthly (or weekly) spend roll-up from the synced wallet
- `slot-watch --weekday do --before 18:00` — polls slots, exits 0 the moment one matches
- `since 7d` — counts of new deliveries / wallet entries / cart edits in a window
- `sql "SELECT ..."` — read-only SQL escape hatch against your local Picnic store
- `drift` — price changes across local article snapshots
- `export` — dump the entire local store as JSON / CSV / NDJSON

### For agents
- `--json` + `--select <dotted.path>` on every command — narrow output, no context bloat
- `--data-source local|live|auto` — choose between the local store and live API
- `--dry-run` on every mutation — preview without executing
- `agent-context` — JSON description of the full command tree for agent discovery
- `which <capability>` — find the command for a use-case
- An MCP server at `cmd/picnic-pp-mcp` — Claude Desktop / Cursor / agents can call every command above as MCP tools

### Multi-country
- `auth login --country nl|de|fr` — switch base URL between Picnic NL / DE / FR

---

## Install

This is a personal fork. Clone and build directly:

```bash
git clone https://github.com/Pimmetjeoss/picnic-cli.git
cd picnic-cli
go build -o picnic-pp-cli ./cmd/picnic-pp-cli      # on Windows: picnic-pp-cli.exe
./picnic-pp-cli --version
```

Requires Go 1.26.3 or newer. Optional: copy `picnic-pp-cli` (or `.exe`) somewhere on your `PATH`.

### MCP server (Claude Desktop / Cursor / agents)

```bash
go build -o picnic-pp-mcp ./cmd/picnic-pp-mcp
```

Add the MCP binary to your MCP host config and every command above becomes an agent tool automatically.

## Authentication

Picnic has no API keys. `picnic-pp-cli auth login --email <your-email> --password '...'` MD5-hashes your password, POSTs `/user/login`, and stores the `x-picnic-auth` token in `~/.config/picnic-pp-cli/config.toml`. If Picnic challenges with 2FA SMS, run `picnic-pp-cli auth verify-sms <code>` to finish the handshake. After that every command picks up the stored token automatically.

## Quick Start

```bash
# One-time login; stores x-picnic-auth in config.
picnic-pp-cli auth login --email <your-email> --password '...' 


# Confirm auth + base URL + local store are healthy.
picnic-pp-cli doctor --json


# Pull cart, user, delivery slots, full deliveries, and wallet pages into the
# local SQLite store. Prefer `picnic-sync` over the generic `sync` — only
# `picnic-sync` uses the correct HTTP method/body shape per endpoint.
picnic-pp-cli picnic-sync


# Live search against the Picnic API with narrowed output.
picnic-pp-cli articles find --term 'havermelk' --json --select id,name,price


# Same query, but offline, against the synced store — for tight agent loops.
picnic-pp-cli search 'havermelk' --json


# Roll up wallet transactions per month — answers 'where did my groceries money go'.
picnic-pp-cli spend-trend --by month --since 2026-01-01 --json

```

## Unique Features

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

## Usage

Run `picnic-pp-cli --help` for the full command reference and flag list.

## Commands

### articles

Picnic product catalogue: search, suggest, and inspect articles

- **`picnic-pp-cli articles category`** - Get the category an article belongs to
- **`picnic-pp-cli articles find`** - Search Picnic articles by keyword (live API)
- **`picnic-pp-cli articles get`** - Get full detail for a single article (product)
- **`picnic-pp-cli articles suggest`** - Get product suggestions for a partial query

### cart

Active shopping cart for the authenticated user

- **`picnic-pp-cli cart add`** - Add an article to the cart
- **`picnic-pp-cli cart checkout`** - Confirm and place the order for the current cart
- **`picnic-pp-cli cart clear`** - Remove all items from the cart
- **`picnic-pp-cli cart get`** - Get the current cart with items, totals, and selected delivery slot
- **`picnic-pp-cli cart remove`** - Remove an article from the cart
- **`picnic-pp-cli cart set_slot`** - Reserve a delivery slot for the current cart

### categories

Browse Picnic's product category hierarchy

- **`picnic-pp-cli categories list`** - Get the product category tree with configurable depth

### deliveries

Past and active deliveries

- **`picnic-pp-cli deliveries cancel`** - Cancel a scheduled delivery (only before cutoff)
- **`picnic-pp-cli deliveries get`** - Get a specific delivery
- **`picnic-pp-cli deliveries list`** - List deliveries; pass --summary for the lightweight payload
- **`picnic-pp-cli deliveries position`** - Get the live driver position for an in-flight delivery
- **`picnic-pp-cli deliveries rate`** - Submit a 0-10 rating for a completed delivery
- **`picnic-pp-cli deliveries resend_invoice`** - Resend the delivery invoice email
- **`picnic-pp-cli deliveries scenario`** - Get delivery route/scenario detail (driver, ETA window)

### delivery_slots

Available delivery time windows

- **`picnic-pp-cli delivery_slots list`** - List delivery slots currently bookable from this cart

### lists

Shopping lists (saved product collections)

- **`picnic-pp-cli lists get`** - Get a specific shopping list with its items
- **`picnic-pp-cli lists list`** - List all shopping lists

### mgm

Member-get-member (referral) program

- **`picnic-pp-cli mgm get`** - Get referral program details and your invite link

### payment

Payment profile and wallet

- **`picnic-pp-cli payment profile`** - Get the user's payment profile and methods
- **`picnic-pp-cli payment wallet`** - Paginated wallet transaction history
- **`picnic-pp-cli payment wallet_detail`** - Get full breakdown of a single wallet transaction

### user

Authenticated Picnic user profile and account details

- **`picnic-pp-cli user get`** - Get the authenticated user's profile, address, and household
- **`picnic-pp-cli user get-settings`** - Get user settings and feature flags for the authenticated user


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
picnic-pp-cli articles get mock-value

# JSON for scripting and agents
picnic-pp-cli articles get mock-value --json

# Filter to specific fields
picnic-pp-cli articles get mock-value --json --select id,name,status

# Dry run — show the request without sending
picnic-pp-cli articles get mock-value --dry-run

# Agent mode — JSON + compact + no prompts in one flag
picnic-pp-cli articles get mock-value --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Use with Claude Code

Install the focused skill — it auto-installs the CLI on first invocation:

```bash
npx skills add mvanhorn/printing-press-library/cli-skills/pp-picnic -g
```

Then invoke `/pp-picnic <query>` in Claude Code. The skill is the most efficient path — Claude Code drives the CLI directly without an MCP server in the middle.

<details>
<summary>Use as an MCP server in Claude Code (advanced)</summary>

If you'd rather register this CLI as an MCP server in Claude Code, install the MCP binary first:


```bash
go install github.com/mvanhorn/printing-press-library/library/shopping/picnic/cmd/picnic-pp-mcp@latest
```

Then register it:

```bash
claude mcp add picnic picnic-pp-mcp -e PICNIC_AUTH_KEY=<your-key>
```

</details>

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/picnic-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `PICNIC_AUTH_KEY` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


```bash
go install github.com/mvanhorn/printing-press-library/library/shopping/picnic/cmd/picnic-pp-mcp@latest
```

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "picnic": {
      "command": "picnic-pp-mcp",
      "env": {
        "PICNIC_AUTH_KEY": "<your-key>"
      }
    }
  }
}
```

</details>

## Health Check

```bash
picnic-pp-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/picnic-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `PICNIC_AUTH_KEY` | per_call | Yes | Set to your API credential. |

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `picnic-pp-cli doctor` to check credentials
- Verify the environment variable is set: `echo $PICNIC_AUTH_KEY`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **auth login returns 401** — Double-check email/password; Picnic uses your app login. If you have 2FA enabled, run `auth login --send-sms` and then `auth verify-sms <code>`.
- **Calls return 403 after a while** — Token may have been invalidated server-side. Run `auth login` again.
- **search-local returns nothing** — Run `sync` first — search-local only sees articles you've previously fetched.
- **country mismatch (DE/FR user)** — Run `picnic-pp-cli country de` (or `fr`) to switch the base URL.

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**MikeBrink/python-picnic-api**](https://github.com/MikeBrink/python-picnic-api) — Python (190 stars)
- [**MRVDH/picnic-api**](https://github.com/MRVDH/picnic-api) — TypeScript (80 stars)
- [**ivo-toby/mcp-picnic**](https://github.com/ivo-toby/mcp-picnic) — TypeScript (54 stars)
- [**simonmartyr/picnic-api**](https://github.com/simonmartyr/picnic-api) — Go (10 stars)

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
