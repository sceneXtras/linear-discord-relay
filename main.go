package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const linearAPIURL = "https://api.linear.app/graphql"

// ============================================================================
// LINEAR TYPES
// ============================================================================

type GraphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type GraphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []GraphQLError  `json:"errors,omitempty"`
}

type GraphQLError struct {
	Message string `json:"message"`
}

type IssuesResponse struct {
	Issues struct {
		Nodes    []Issue  `json:"nodes"`
		PageInfo PageInfo `json:"pageInfo"`
	} `json:"issues"`
}

type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type Issue struct {
	ID            string    `json:"id"`
	Identifier    string    `json:"identifier"`
	Title         string    `json:"title"`
	Description   string    `json:"description,omitempty"`
	Priority      int       `json:"priority"`
	PriorityLabel string    `json:"priorityLabel"`
	URL           string    `json:"url"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	State         State     `json:"state"`
	Assignee      *User     `json:"assignee"`
	Team          Team      `json:"team"`
	Labels        struct {
		Nodes []Label `json:"nodes"`
	} `json:"labels"`
}

type State struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Type  string `json:"type"`
}

type User struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

type Team struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Linear Webhook types
type LinearWebhook struct {
	Action       string          `json:"action"`
	Actor        *User           `json:"actor,omitempty"`
	CreatedAt    string          `json:"createdAt"`
	Data         json.RawMessage `json:"data"`
	Type         string          `json:"type"`
	URL          string          `json:"url,omitempty"`
	UpdatedFrom  json.RawMessage `json:"updatedFrom,omitempty"`
	WebhookID    string          `json:"webhookId,omitempty"`
	WebhookTS    int64           `json:"webhookTimestamp,omitempty"`
}

type LinearWebhookIssue struct {
	ID            string  `json:"id"`
	Identifier    string  `json:"identifier"`
	Title         string  `json:"title"`
	Description   string  `json:"description,omitempty"`
	Priority      int     `json:"priority"`
	PriorityLabel string  `json:"priorityLabel,omitempty"`
	State         *State  `json:"state,omitempty"`
	Assignee      *User   `json:"assignee,omitempty"`
	Team          *Team   `json:"team,omitempty"`
	Labels        []Label `json:"labels,omitempty"`
	URL           string  `json:"url,omitempty"`
}

type LinearWebhookComment struct {
	ID        string              `json:"id"`
	Body      string              `json:"body"`
	Issue     *LinearWebhookIssue `json:"issue,omitempty"`
	User      *User               `json:"user,omitempty"`
	CreatedAt string              `json:"createdAt,omitempty"`
	URL       string              `json:"url,omitempty"`
}

type LinearWebhookProject struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	State       string `json:"state,omitempty"`
	URL         string `json:"url,omitempty"`
}

// ============================================================================
// DISCORD TYPES
// ============================================================================

type DiscordWebhook struct {
	Content   string         `json:"content,omitempty"`
	Username  string         `json:"username,omitempty"`
	AvatarURL string         `json:"avatar_url,omitempty"`
	Embeds    []DiscordEmbed `json:"embeds,omitempty"`
}

type DiscordEmbed struct {
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description,omitempty"`
	URL         string         `json:"url,omitempty"`
	Color       int            `json:"color,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
	Footer      *DiscordFooter `json:"footer,omitempty"`
	Author      *DiscordAuthor `json:"author,omitempty"`
	Fields      []DiscordField `json:"fields,omitempty"`
}

type DiscordFooter struct {
	Text    string `json:"text,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
}

type DiscordAuthor struct {
	Name    string `json:"name,omitempty"`
	URL     string `json:"url,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
}

type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// Colors
const (
	ColorBlue   = 0x5E6AD2 // Linear brand color
	ColorGreen  = 0x22C55E // Success/Done
	ColorYellow = 0xEAB308 // Warning/In Progress
	ColorRed    = 0xEF4444 // Error/Urgent
	ColorGray   = 0x6B7280 // Neutral
	ColorPurple = 0x8B5CF6 // Comments
)

const linearAvatarURL = "https://asset.brandfetch.io/ideiLNHwrW/id_xq4rBdb.png"

var (
	linearAPIKey      string
	discordWebhookURL string
)

// ============================================================================
// MAIN
// ============================================================================

func main() {
	discordWebhookURL = os.Getenv("DISCORD_WEBHOOK_URL")
	if discordWebhookURL == "" {
		log.Fatal("DISCORD_WEBHOOK_URL environment variable is required")
	}

	// LINEAR_API_KEY is optional - only needed for daily digest
	linearAPIKey = os.Getenv("LINEAR_API_KEY")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Routes
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/webhook", handleLinearWebhook)     // Linear → Discord relay
	http.HandleFunc("/report", handleReport)             // Daily digest summary
	http.HandleFunc("/report/by-user", handleReportByUser) // Detailed per-user report
	http.HandleFunc("/", handleRoot)

	log.Printf("Linear-Discord Communication Relay listening on port %s", port)
	log.Printf("Endpoints: /webhook (Linear relay), /report (daily digest), /health")
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service": "Linear-Discord Communication Relay",
		"version": "1.0.0",
		"endpoints": map[string]string{
			"/webhook": "POST - Receive Linear webhooks and forward to Discord",
			"/report":  "GET/POST - Generate and send daily digest",
			"/health":  "GET - Health check",
		},
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ============================================================================
// WEBHOOK RELAY (Linear → Discord)
// ============================================================================

func handleLinearWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		http.Error(w, "Error reading request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	log.Printf("Received Linear webhook: %s", string(body))

	var webhook LinearWebhook
	if err := json.Unmarshal(body, &webhook); err != nil {
		log.Printf("Error parsing webhook: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	discordPayload, err := transformWebhookToDiscord(webhook)
	if err != nil {
		log.Printf("Error transforming webhook: %v", err)
		http.Error(w, "Error processing webhook", http.StatusInternalServerError)
		return
	}

	if discordPayload == nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := sendToDiscord(discordPayload); err != nil {
		log.Printf("Error sending to Discord: %v", err)
		http.Error(w, "Error forwarding to Discord", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "forwarded"})
}

func transformWebhookToDiscord(webhook LinearWebhook) (*DiscordWebhook, error) {
	switch webhook.Type {
	case "Issue":
		return transformIssueWebhook(webhook)
	case "Comment":
		return transformCommentWebhook(webhook)
	case "Project":
		return transformProjectWebhook(webhook)
	default:
		log.Printf("Unhandled webhook type: %s", webhook.Type)
		return nil, nil
	}
}

func transformIssueWebhook(webhook LinearWebhook) (*DiscordWebhook, error) {
	var issue LinearWebhookIssue
	if err := json.Unmarshal(webhook.Data, &issue); err != nil {
		return nil, fmt.Errorf("failed to parse issue data: %w", err)
	}

	var title, emoji string
	color := ColorBlue

	switch webhook.Action {
	case "create":
		emoji = "🎯"
		title = "New Issue Created"
		color = ColorBlue
	case "update":
		emoji = "📝"
		title = "Issue Updated"
		color = ColorYellow
	case "remove":
		emoji = "🗑️"
		title = "Issue Removed"
		color = ColorRed
	default:
		emoji = "📋"
		title = fmt.Sprintf("Issue %s", strings.Title(webhook.Action))
	}

	description := truncate(issue.Description, 300)
	if description == "" {
		description = "*No description*"
	}

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("%s %s", emoji, title),
		Description: fmt.Sprintf("**[%s](%s)** - %s\n\n%s", issue.Identifier, issue.URL, issue.Title, description),
		URL:         issue.URL,
		Color:       color,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Fields:      []DiscordField{},
	}

	if issue.State != nil {
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "Status",
			Value:  fmt.Sprintf("%s %s", getStateEmoji(issue.State.Type), issue.State.Name),
			Inline: true,
		})
	}

	if issue.PriorityLabel != "" {
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "Priority",
			Value:  fmt.Sprintf("%s %s", getPriorityEmoji(issue.Priority), issue.PriorityLabel),
			Inline: true,
		})
	}

	if issue.Assignee != nil {
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "Assignee",
			Value:  fmt.Sprintf("👤 %s", issue.Assignee.Name),
			Inline: true,
		})
	}

	if issue.Team != nil {
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "Team",
			Value:  fmt.Sprintf("👥 %s", issue.Team.Name),
			Inline: true,
		})
	}

	if len(issue.Labels) > 0 {
		labelNames := make([]string, len(issue.Labels))
		for i, label := range issue.Labels {
			labelNames[i] = fmt.Sprintf("`%s`", label.Name)
		}
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "Labels",
			Value:  strings.Join(labelNames, " "),
			Inline: false,
		})
	}

	if webhook.Actor != nil {
		embed.Footer = &DiscordFooter{Text: fmt.Sprintf("by %s", webhook.Actor.Name)}
	}

	return &DiscordWebhook{
		Username:  "Linear",
		AvatarURL: linearAvatarURL,
		Embeds:    []DiscordEmbed{embed},
	}, nil
}

func transformCommentWebhook(webhook LinearWebhook) (*DiscordWebhook, error) {
	var comment LinearWebhookComment
	if err := json.Unmarshal(webhook.Data, &comment); err != nil {
		return nil, fmt.Errorf("failed to parse comment data: %w", err)
	}

	var title, emoji string

	switch webhook.Action {
	case "create":
		emoji = "💬"
		title = "New Comment"
	case "update":
		emoji = "✏️"
		title = "Comment Updated"
	case "remove":
		emoji = "🗑️"
		title = "Comment Removed"
	default:
		emoji = "💬"
		title = fmt.Sprintf("Comment %s", strings.Title(webhook.Action))
	}

	issueInfo := ""
	if comment.Issue != nil {
		issueInfo = fmt.Sprintf("**[%s](%s)** - %s", comment.Issue.Identifier, comment.Issue.URL, comment.Issue.Title)
	}

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("%s %s", emoji, title),
		Description: fmt.Sprintf("%s\n\n>>> %s", issueInfo, truncate(comment.Body, 500)),
		URL:         comment.URL,
		Color:       ColorPurple,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	if comment.User != nil {
		embed.Author = &DiscordAuthor{Name: comment.User.Name}
	}

	if webhook.Actor != nil {
		embed.Footer = &DiscordFooter{Text: fmt.Sprintf("by %s", webhook.Actor.Name)}
	}

	return &DiscordWebhook{
		Username:  "Linear",
		AvatarURL: linearAvatarURL,
		Embeds:    []DiscordEmbed{embed},
	}, nil
}

func transformProjectWebhook(webhook LinearWebhook) (*DiscordWebhook, error) {
	var project LinearWebhookProject
	if err := json.Unmarshal(webhook.Data, &project); err != nil {
		return nil, fmt.Errorf("failed to parse project data: %w", err)
	}

	var title, emoji string
	color := ColorBlue

	switch webhook.Action {
	case "create":
		emoji = "🚀"
		title = "New Project Created"
		color = ColorGreen
	case "update":
		emoji = "📊"
		title = "Project Updated"
		color = ColorYellow
	case "remove":
		emoji = "🗑️"
		title = "Project Removed"
		color = ColorRed
	default:
		emoji = "📁"
		title = fmt.Sprintf("Project %s", strings.Title(webhook.Action))
	}

	description := truncate(project.Description, 300)
	if description == "" {
		description = "*No description*"
	}

	embed := DiscordEmbed{
		Title:       fmt.Sprintf("%s %s", emoji, title),
		Description: fmt.Sprintf("**%s**\n\n%s", project.Name, description),
		URL:         project.URL,
		Color:       color,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Fields:      []DiscordField{},
	}

	if project.State != "" {
		embed.Fields = append(embed.Fields, DiscordField{
			Name:   "State",
			Value:  project.State,
			Inline: true,
		})
	}

	if webhook.Actor != nil {
		embed.Footer = &DiscordFooter{Text: fmt.Sprintf("by %s", webhook.Actor.Name)}
	}

	return &DiscordWebhook{
		Username:  "Linear",
		AvatarURL: linearAvatarURL,
		Embeds:    []DiscordEmbed{embed},
	}, nil
}

// ============================================================================
// DAILY DIGEST
// ============================================================================

func handleReport(w http.ResponseWriter, r *http.Request) {
	if linearAPIKey == "" {
		http.Error(w, "LINEAR_API_KEY not configured", http.StatusServiceUnavailable)
		return
	}

	if err := generateAndSendReport(); err != nil {
		log.Printf("Error generating report: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "report_sent"})
}

func generateAndSendReport() error {
	log.Println("Fetching issues from Linear...")
	issues, err := fetchAllOpenIssues()
	if err != nil {
		return fmt.Errorf("failed to fetch issues: %w", err)
	}

	log.Printf("Fetched %d open issues", len(issues))

	if len(issues) == 0 {
		return sendNoIssuesReport()
	}

	byStatus := groupByStatus(issues)
	byAssignee := groupByAssignee(issues)

	return sendReport(issues, byStatus, byAssignee)
}

func fetchAllOpenIssues() ([]Issue, error) {
	var allIssues []Issue
	var cursor string

	query := `
		query($cursor: String) {
			issues(
				filter: {
					state: { type: { nin: ["completed", "canceled"] } }
				}
				first: 100
				after: $cursor
				orderBy: updatedAt
			) {
				nodes {
					id
					identifier
					title
					priority
					priorityLabel
					url
					createdAt
					updatedAt
					state {
						id
						name
						color
						type
					}
					assignee {
						id
						name
						displayName
						email
					}
					team {
						id
						name
						key
					}
					labels {
						nodes {
							id
							name
							color
						}
					}
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	`

	for {
		variables := map[string]interface{}{}
		if cursor != "" {
			variables["cursor"] = cursor
		}

		resp, err := executeGraphQL(query, variables)
		if err != nil {
			return nil, err
		}

		var issuesResp IssuesResponse
		if err := json.Unmarshal(resp, &issuesResp); err != nil {
			return nil, fmt.Errorf("failed to parse issues response: %w", err)
		}

		allIssues = append(allIssues, issuesResp.Issues.Nodes...)

		if !issuesResp.Issues.PageInfo.HasNextPage {
			break
		}
		cursor = issuesResp.Issues.PageInfo.EndCursor
	}

	return allIssues, nil
}

func executeGraphQL(query string, variables map[string]interface{}) (json.RawMessage, error) {
	reqBody := GraphQLRequest{Query: query, Variables: variables}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", linearAPIURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", linearAPIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linear API returned status %d: %s", resp.StatusCode, string(body))
	}

	var graphResp GraphQLResponse
	if err := json.Unmarshal(body, &graphResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(graphResp.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL errors: %v", graphResp.Errors)
	}

	return graphResp.Data, nil
}

type StatusGroup struct {
	Name   string
	Type   string
	Color  string
	Issues []Issue
}

type AssigneeGroup struct {
	Name   string
	Issues []Issue
}

func groupByStatus(issues []Issue) []StatusGroup {
	groups := make(map[string]*StatusGroup)

	for _, issue := range issues {
		key := issue.State.Name
		if groups[key] == nil {
			groups[key] = &StatusGroup{
				Name:   issue.State.Name,
				Type:   issue.State.Type,
				Color:  issue.State.Color,
				Issues: []Issue{},
			}
		}
		groups[key].Issues = append(groups[key].Issues, issue)
	}

	result := make([]StatusGroup, 0, len(groups))
	for _, g := range groups {
		result = append(result, *g)
	}

	sort.Slice(result, func(i, j int) bool {
		return getStatusPriority(result[i].Type) < getStatusPriority(result[j].Type)
	})

	return result
}

func getStatusPriority(statusType string) int {
	switch statusType {
	case "started":
		return 1
	case "unstarted":
		return 2
	case "backlog":
		return 3
	default:
		return 4
	}
}

func groupByAssignee(issues []Issue) []AssigneeGroup {
	groups := make(map[string]*AssigneeGroup)

	for _, issue := range issues {
		var name string
		if issue.Assignee != nil {
			name = issue.Assignee.Name
			if issue.Assignee.DisplayName != "" {
				name = issue.Assignee.DisplayName
			}
		} else {
			name = "Unassigned"
		}

		if groups[name] == nil {
			groups[name] = &AssigneeGroup{Name: name, Issues: []Issue{}}
		}
		groups[name].Issues = append(groups[name].Issues, issue)
	}

	result := make([]AssigneeGroup, 0, len(groups))
	for _, g := range groups {
		result = append(result, *g)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == "Unassigned" {
			return false
		}
		if result[j].Name == "Unassigned" {
			return true
		}
		return len(result[i].Issues) > len(result[j].Issues)
	})

	return result
}

func sendNoIssuesReport() error {
	embed := DiscordEmbed{
		Title:       "📊 Linear Daily Digest",
		Description: "No open issues found. Great job keeping the backlog clean! 🎉",
		Color:       ColorGreen,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Footer:      &DiscordFooter{Text: "Linear Daily Digest"},
	}

	return sendToDiscord(&DiscordWebhook{
		Username:  "Linear Daily Digest",
		AvatarURL: linearAvatarURL,
		Embeds:    []DiscordEmbed{embed},
	})
}

func sendReport(issues []Issue, byStatus []StatusGroup, byAssignee []AssigneeGroup) error {
	urgentCount := 0
	highCount := 0
	for _, issue := range issues {
		if issue.Priority == 1 {
			urgentCount++
		} else if issue.Priority == 2 {
			highCount++
		}
	}

	var statusLines []string
	for _, group := range byStatus {
		statusLines = append(statusLines, fmt.Sprintf("%s **%s**: %d", getStateEmoji(group.Type), group.Name, len(group.Issues)))
	}

	var assigneeLines []string
	for _, group := range byAssignee {
		emoji := "👤"
		if group.Name == "Unassigned" {
			emoji = "❓"
		}
		assigneeLines = append(assigneeLines, fmt.Sprintf("%s **%s**: %d", emoji, group.Name, len(group.Issues)))
	}

	var priorityAlerts []string
	if urgentCount > 0 {
		priorityAlerts = append(priorityAlerts, fmt.Sprintf("🔴 **%d Urgent**", urgentCount))
	}
	if highCount > 0 {
		priorityAlerts = append(priorityAlerts, fmt.Sprintf("🟠 **%d High Priority**", highCount))
	}

	mainEmbed := DiscordEmbed{
		Title:     "📊 Linear Daily Digest",
		Color:     ColorBlue,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Footer:    &DiscordFooter{Text: fmt.Sprintf("Total: %d open issues • Generated at", len(issues))},
		Fields:    []DiscordField{},
	}

	summaryParts := []string{fmt.Sprintf("**%d** open issues across your workspace", len(issues))}
	if len(priorityAlerts) > 0 {
		summaryParts = append(summaryParts, strings.Join(priorityAlerts, " | "))
	}
	mainEmbed.Description = strings.Join(summaryParts, "\n")

	mainEmbed.Fields = append(mainEmbed.Fields,
		DiscordField{Name: "📋 By Status", Value: strings.Join(statusLines, "\n"), Inline: true},
		DiscordField{Name: "👥 By Assignee", Value: strings.Join(assigneeLines, "\n"), Inline: true},
	)

	embeds := []DiscordEmbed{mainEmbed}

	if urgentCount > 0 || highCount > 0 {
		var priorityIssues []string
		count := 0
		for _, issue := range issues {
			if issue.Priority <= 2 && count < 10 {
				emoji := "🔴"
				if issue.Priority == 2 {
					emoji = "🟠"
				}
				assignee := "Unassigned"
				if issue.Assignee != nil {
					assignee = issue.Assignee.Name
				}
				priorityIssues = append(priorityIssues, fmt.Sprintf("%s [**%s**](%s) - %s (%s)",
					emoji, issue.Identifier, issue.URL, truncate(issue.Title, 40), assignee))
				count++
			}
		}

		if len(priorityIssues) > 0 {
			embeds = append(embeds, DiscordEmbed{
				Title:       "🚨 Priority Issues",
				Description: strings.Join(priorityIssues, "\n"),
				Color:       ColorRed,
			})
		}
	}

	today := time.Now().Truncate(24 * time.Hour)
	var recentIssues []string
	for _, issue := range issues {
		if issue.UpdatedAt.After(today) && len(recentIssues) < 5 {
			recentIssues = append(recentIssues, fmt.Sprintf("• [**%s**](%s) - %s",
				issue.Identifier, issue.URL, truncate(issue.Title, 50)))
		}
	}

	if len(recentIssues) > 0 {
		embeds = append(embeds, DiscordEmbed{
			Title:       "🔄 Recently Updated",
			Description: strings.Join(recentIssues, "\n"),
			Color:       ColorYellow,
		})
	}

	return sendToDiscord(&DiscordWebhook{
		Username:  "Linear Daily Digest",
		AvatarURL: linearAvatarURL,
		Embeds:    embeds,
	})
}

// ============================================================================
// PER-USER REPORT
// ============================================================================

func handleReportByUser(w http.ResponseWriter, r *http.Request) {
	if linearAPIKey == "" {
		http.Error(w, "LINEAR_API_KEY not configured", http.StatusServiceUnavailable)
		return
	}

	if err := generateUserTasksReport(); err != nil {
		log.Printf("Error generating user report: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "user_report_sent"})
}

func generateUserTasksReport() error {
	log.Println("Fetching issues for per-user report...")
	issues, err := fetchAllOpenIssues()
	if err != nil {
		return fmt.Errorf("failed to fetch issues: %w", err)
	}

	if len(issues) == 0 {
		return sendNoIssuesReport()
	}

	byAssignee := groupByAssignee(issues)

	// Send one embed per user (Discord limit: 10 embeds per message)
	var embeds []DiscordEmbed

	// Header embed
	embeds = append(embeds, DiscordEmbed{
		Title:       "📋 Open Tasks by User",
		Description: fmt.Sprintf("**%d** open tasks across **%d** assignees", len(issues), len(byAssignee)),
		Color:       ColorBlue,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	})

	// Per-user embeds
	for _, group := range byAssignee {
		var taskLines []string
		for i, issue := range group.Issues {
			if i >= 15 { // Limit to 15 tasks per user
				taskLines = append(taskLines, fmt.Sprintf("*... and %d more*", len(group.Issues)-15))
				break
			}
			priorityEmoji := getPriorityEmoji(issue.Priority)
			taskLines = append(taskLines, fmt.Sprintf("%s [%s](%s) - %s",
				priorityEmoji, issue.Identifier, issue.URL, truncate(issue.Title, 50)))
		}

		emoji := "👤"
		if group.Name == "Unassigned" {
			emoji = "❓"
		}

		embeds = append(embeds, DiscordEmbed{
			Title:       fmt.Sprintf("%s %s (%d tasks)", emoji, group.Name, len(group.Issues)),
			Description: strings.Join(taskLines, "\n"),
			Color:       ColorGray,
		})

		// Discord limit: 10 embeds per message, send in batches
		if len(embeds) >= 10 {
			if err := sendToDiscord(&DiscordWebhook{
				Username:  "Linear Task Report",
				AvatarURL: linearAvatarURL,
				Embeds:    embeds,
			}); err != nil {
				return err
			}
			embeds = nil
			time.Sleep(500 * time.Millisecond) // Rate limit
		}
	}

	// Send remaining embeds
	if len(embeds) > 0 {
		return sendToDiscord(&DiscordWebhook{
			Username:  "Linear Task Report",
			AvatarURL: linearAvatarURL,
			Embeds:    embeds,
		})
	}

	return nil
}

// ============================================================================
// HELPERS
// ============================================================================

func sendToDiscord(payload *DiscordWebhook) error {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal discord payload: %w", err)
	}

	log.Printf("Sending to Discord: %d embeds", len(payload.Embeds))

	resp, err := http.Post(discordWebhookURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send to discord: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord returned status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("Successfully sent to Discord (status: %d)", resp.StatusCode)
	return nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func getStateEmoji(stateType string) string {
	switch stateType {
	case "backlog":
		return "📥"
	case "unstarted":
		return "⚪"
	case "started":
		return "🔵"
	case "completed":
		return "✅"
	case "canceled":
		return "❌"
	default:
		return "📋"
	}
}

func getPriorityEmoji(priority int) string {
	switch priority {
	case 0:
		return "⬜"
	case 1:
		return "🔴"
	case 2:
		return "🟠"
	case 3:
		return "🟡"
	case 4:
		return "🟢"
	default:
		return "⬜"
	}
}
