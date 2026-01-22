# Linear-Discord Communication Relay

A unified Go service that bridges Linear and Discord with two main features:

1. **Webhook Relay**: Receives Linear webhooks (issues, comments, projects) and forwards them as formatted Discord embeds
2. **Daily Digest**: Generates daily reports of open issues grouped by status and assignee

**Deployed at**: `communication-relay.scenextras.com`

## Features

### Webhook Relay (`/webhook`)
- **Issue Events**: New issues, updates, removals with priority, status, assignee, labels
- **Comment Events**: New comments with quoted content and issue context
- **Project Events**: Project creation, updates, removals
- **Rich Embeds**: Color-coded cards with emojis, fields, and timestamps

### Daily Digest (`/report`)
- **Status Breakdown**: Issues grouped by workflow state (In Progress, Todo, Backlog)
- **Assignee Breakdown**: Issues grouped by team member
- **Priority Alerts**: Highlights urgent and high-priority issues
- **Recent Activity**: Shows recently updated issues

## Discord Preview

**Webhook Relay:**
```
🎯 New Issue Created
[SCE-239](url) - Fix authentication bug

*Issue description here...*

Status: 🔵 In Progress | Priority: 🟠 High | Assignee: 👤 John
Team: 👥 Backend | Labels: `bug` `auth`

by Jane Smith • Just now
```

**Daily Digest:**
```
📊 Linear Daily Digest
42 open issues across your workspace
🔴 3 Urgent | 🟠 8 High Priority

📋 By Status          👥 By Assignee
🔵 In Progress: 12    👤 John: 15
⚪ Todo: 18           👤 Jane: 12
📥 Backlog: 12        ❓ Unassigned: 7
```

## Quick Start

### Environment Variables

```bash
# Required
DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/...

# Required for daily digest only
LINEAR_API_KEY=lin_api_...

# Optional
PORT=8080
```

### Run Locally

```bash
export DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/..."
export LINEAR_API_KEY="lin_api_..."
go run main.go
```

### Docker

```bash
docker-compose up -d
```

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Service info and available endpoints |
| `/health` | GET | Health check |
| `/webhook` | POST | Receive Linear webhooks → forward to Discord |
| `/report` | GET/POST | Generate and send daily digest |

## Setup

### 1. Linear Webhook Configuration

1. Go to [Linear Settings → API → Webhooks](https://linear.app/settings/api)
2. Create new webhook:
   - **Label**: Discord Communication Relay
   - **URL**: `https://communication-relay.scenextras.com/webhook`
   - **Events**: Issues, Comments, Project Updates
3. Enable the webhook

### 2. Linear API Key (for Daily Digest)

1. Go to [Linear Settings → API](https://linear.app/settings/api)
2. Click "Create key"
3. Copy the key (starts with `lin_api_`)

### 3. Daily Digest Schedule

The daily digest runs via GitHub Actions at 9 AM UTC on weekdays. You can also trigger manually:

```bash
curl https://communication-relay.scenextras.com/report
```

## Deployment

### Dokku (Production)

```bash
# Create app
dokku apps:create linear-daily-digest

# Set environment variables
dokku config:set linear-daily-digest \
  DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/..." \
  LINEAR_API_KEY="lin_api_..."

# Deploy via git push (GitHub Actions handles this)
git push dokku main
```

### Manual Deployment

```bash
# Build
go build -o linear-daily-digest .

# Run
./linear-daily-digest
```

## Testing

```bash
# Health check
curl https://communication-relay.scenextras.com/health

# Test webhook relay
curl -X POST https://communication-relay.scenextras.com/webhook \
  -H "Content-Type: application/json" \
  -d '{
    "action": "create",
    "type": "Issue",
    "data": {
      "identifier": "TEST-1",
      "title": "Test Issue",
      "url": "https://linear.app/test",
      "priority": 2,
      "priorityLabel": "High"
    },
    "actor": {"name": "Test User"}
  }'

# Trigger daily digest
curl https://communication-relay.scenextras.com/report
```

## GitHub Actions

### Deployment (`.github/workflows/deploy.yml`)
- Triggers on push to `main`
- Deploys to Dokku via SSH
- Verifies deployment with health check

### Daily Digest (`.github/workflows/daily-digest.yml`)
- Runs at 9 AM UTC on weekdays (Mon-Fri)
- Can be triggered manually via workflow_dispatch

## Customization

Edit `main.go` to customize:

- **Colors**: Modify `Color*` constants
- **Emojis**: Modify `getStateEmoji()` and `getPriorityEmoji()`
- **Fields**: Add/remove fields in transform functions
- **Filters**: Modify GraphQL query to filter by team, label, etc.

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

## Architecture

```
┌─────────────────┐     ┌─────────────────────────────────────┐
│  Linear         │     │  Communication Relay                │
│  Webhooks       │────▶│  (communication-relay.scenextras)   │
└─────────────────┘     │                                     │
                        │  /webhook  → Transform → Discord    │
┌─────────────────┐     │  /report   → Fetch → Format → Discord│
│  GitHub Actions │────▶│  /health   → Health Check           │
│  (cron)         │     └──────────────────┬──────────────────┘
└─────────────────┘                        │
                                           ▼
                        ┌─────────────────────────────────────┐
                        │  Discord Webhook                    │
                        │  (Integrations Channel)             │
                        └─────────────────────────────────────┘
```

## License

MIT
