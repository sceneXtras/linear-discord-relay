# Linear Daily Digest

A minimal Go service that generates daily reports of open Linear issues grouped by status and assignee, sending beautifully formatted Discord embeds.

## Features

- **Status Breakdown**: Issues grouped by workflow state (In Progress, Todo, Backlog, etc.)
- **Assignee Breakdown**: Issues grouped by team member
- **Priority Alerts**: Highlights urgent and high-priority issues
- **Recent Activity**: Shows recently updated issues
- **Rich Embeds**: Color-coded cards with emojis and clickable links
- **Dual Mode**: Run as one-shot (cron) or HTTP server

## Discord Report Preview

```
📊 Linear Daily Digest
━━━━━━━━━━━━━━━━━━━━━━━━
42 open issues across your workspace
🔴 3 Urgent | 🟠 8 High Priority

📋 By Status          👥 By Assignee
🔵 In Progress: 12    👤 John: 15
⚪ Todo: 18           👤 Jane: 12
📥 Backlog: 12        👤 Bob: 8
                      ❓ Unassigned: 7

🚨 Priority Issues
🔴 SCE-234 - Fix auth bug (John)
🔴 SCE-241 - Database timeout (Jane)
🟠 SCE-239 - Update API docs (Unassigned)
...

🔄 Recently Updated
• SCE-245 - Add dark mode toggle
• SCE-243 - Refactor user service
...
```

## Quick Start

### Option 1: One-Shot (Cron Job)

```bash
# Run once and exit
export LINEAR_API_KEY="lin_api_..."
export DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/..."
go run main.go
```

Add to crontab for daily reports:
```bash
# Run daily at 9 AM
0 9 * * * cd /path/to/linear-daily-digest && LINEAR_API_KEY=... DISCORD_WEBHOOK_URL=... ./linear-daily-digest
```

### Option 2: HTTP Server

```bash
# Run as server (trigger via HTTP)
export LINEAR_API_KEY="lin_api_..."
export DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/..."
export MODE=server
go run main.go

# Trigger report manually
curl http://localhost:8080/report
```

### Option 3: Docker

```bash
# Set environment variables
export LINEAR_API_KEY="lin_api_..."
export DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/..."

# Run with docker-compose (server mode)
docker-compose up -d

# Or run one-shot with Docker
docker build -t linear-daily-digest .
docker run --rm \
  -e LINEAR_API_KEY="$LINEAR_API_KEY" \
  -e DISCORD_WEBHOOK_URL="$DISCORD_WEBHOOK_URL" \
  linear-daily-digest
```

## Deployment

### Dokku

```bash
# Create app
dokku apps:create linear-daily-digest

# Set environment variables
dokku config:set linear-daily-digest \
  LINEAR_API_KEY="lin_api_..." \
  DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/..." \
  MODE=server

# Deploy
git push dokku main

# Set up daily cron (on Dokku server)
dokku cron:add linear-daily-digest daily-report "0 9 * * *" "/root/linear-daily-digest"
```

### GitHub Actions (Scheduled)

Create `.github/workflows/daily-digest.yml`:

```yaml
name: Daily Linear Digest

on:
  schedule:
    - cron: '0 9 * * *'  # 9 AM UTC daily
  workflow_dispatch:  # Manual trigger

jobs:
  digest:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Send Daily Digest
        env:
          LINEAR_API_KEY: ${{ secrets.LINEAR_API_KEY }}
          DISCORD_WEBHOOK_URL: ${{ secrets.DISCORD_WEBHOOK_URL }}
        run: go run main.go
```

### Railway / Render

1. Connect GitHub repo
2. Set environment variables: `LINEAR_API_KEY`, `DISCORD_WEBHOOK_URL`, `MODE=server`
3. Deploy
4. Use their cron feature or external trigger

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `LINEAR_API_KEY` | Yes | - | Linear API key (starts with `lin_api_`) |
| `DISCORD_WEBHOOK_URL` | Yes | - | Discord webhook URL |
| `MODE` | No | (one-shot) | Set to `server` for HTTP mode |
| `PORT` | No | 8080 | Server port (when MODE=server) |
| `TZ` | No | UTC | Timezone for timestamps |

## API Endpoints (Server Mode)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/report` | GET/POST | Trigger digest report |
| `/health` | GET | Health check |

## Getting a Linear API Key

1. Go to [Linear Settings → API](https://linear.app/settings/api)
2. Click "Create key"
3. Give it a name (e.g., "Daily Digest Bot")
4. Copy the key (starts with `lin_api_`)

## Customization

Edit `main.go` to customize:

- **Report timing**: Modify the `today` calculation in `sendReport()`
- **Issue limits**: Change the `10` and `5` limits for priority/recent issues
- **Colors**: Modify `Color*` constants
- **Emojis**: Modify `getStateEmoji()` function
- **Filters**: Modify the GraphQL query to filter by team, label, etc.

### Filter by Team

```graphql
issues(
  filter: {
    state: { type: { nin: ["completed", "canceled"] } }
    team: { key: { eq: "ENG" } }  # Add team filter
  }
  ...
)
```

### Filter by Label

```graphql
issues(
  filter: {
    state: { type: { nin: ["completed", "canceled"] } }
    labels: { name: { in: ["bug", "critical"] } }  # Add label filter
  }
  ...
)
```

## License

MIT
