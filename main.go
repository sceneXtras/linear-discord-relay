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

// Linear GraphQL types
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

// Discord types
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
	Fields      []DiscordField `json:"fields,omitempty"`
}

type DiscordFooter struct {
	Text    string `json:"text,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
}

type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// Colors
const (
	ColorBlue   = 0x5E6AD2
	ColorGreen  = 0x22C55E
	ColorYellow = 0xEAB308
	ColorRed    = 0xEF4444
	ColorGray   = 0x6B7280
	ColorPurple = 0x8B5CF6
)

var (
	linearAPIKey      string
	discordWebhookURL string
)

func main() {
	linearAPIKey = os.Getenv("LINEAR_API_KEY")
	if linearAPIKey == "" {
		log.Fatal("LINEAR_API_KEY environment variable is required")
	}

	discordWebhookURL = os.Getenv("DISCORD_WEBHOOK_URL")
	if discordWebhookURL == "" {
		log.Fatal("DISCORD_WEBHOOK_URL environment variable is required")
	}

	// Check if running as server or one-shot (default: server for deployment)
	mode := os.Getenv("MODE")
	if mode == "" {
		mode = "server" // Default to server mode for Dokku deployment
	}
	if mode == "server" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		http.HandleFunc("/report", handleReport)
		http.HandleFunc("/health", handleHealth)
		log.Printf("Linear Daily Digest server listening on port %s", port)
		log.Fatal(http.ListenAndServe(":"+port, nil))
	} else {
		// One-shot mode - run report and exit
		if err := generateAndSendReport(); err != nil {
			log.Fatalf("Failed to generate report: %v", err)
		}
		log.Println("Report sent successfully")
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleReport(w http.ResponseWriter, r *http.Request) {
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

	// Group by status
	byStatus := groupByStatus(issues)

	// Group by assignee
	byAssignee := groupByAssignee(issues)

	// Send report
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
	reqBody := GraphQLRequest{
		Query:     query,
		Variables: variables,
	}

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

	// Convert to slice and sort by status type priority
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
			groups[name] = &AssigneeGroup{
				Name:   name,
				Issues: []Issue{},
			}
		}
		groups[name].Issues = append(groups[name].Issues, issue)
	}

	// Convert to slice and sort by issue count (most issues first)
	result := make([]AssigneeGroup, 0, len(groups))
	for _, g := range groups {
		result = append(result, *g)
	}

	sort.Slice(result, func(i, j int) bool {
		// Unassigned always last
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
		Description: "No open issues found. Great job keeping the backlog clean!",
		Color:       ColorGreen,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Footer: &DiscordFooter{
			Text: "Linear Daily Digest",
		},
	}

	return sendToDiscord(&DiscordWebhook{
		Username:  "Linear Daily Digest",
		AvatarURL: "https://asset.brandfetch.io/ideiLNHwrW/id_xq4rBdb.png",
		Embeds:    []DiscordEmbed{embed},
	})
}

func sendReport(issues []Issue, byStatus []StatusGroup, byAssignee []AssigneeGroup) error {
	// Calculate stats
	urgentCount := 0
	highCount := 0
	for _, issue := range issues {
		if issue.Priority == 1 {
			urgentCount++
		} else if issue.Priority == 2 {
			highCount++
		}
	}

	// Build status summary
	var statusLines []string
	for _, group := range byStatus {
		emoji := getStateEmoji(group.Type)
		statusLines = append(statusLines, fmt.Sprintf("%s **%s**: %d", emoji, group.Name, len(group.Issues)))
	}

	// Build assignee summary
	var assigneeLines []string
	for _, group := range byAssignee {
		emoji := "👤"
		if group.Name == "Unassigned" {
			emoji = "❓"
		}
		assigneeLines = append(assigneeLines, fmt.Sprintf("%s **%s**: %d", emoji, group.Name, len(group.Issues)))
	}

	// Build priority alerts
	var priorityAlerts []string
	if urgentCount > 0 {
		priorityAlerts = append(priorityAlerts, fmt.Sprintf("🔴 **%d Urgent**", urgentCount))
	}
	if highCount > 0 {
		priorityAlerts = append(priorityAlerts, fmt.Sprintf("🟠 **%d High Priority**", highCount))
	}

	// Create main embed
	mainEmbed := DiscordEmbed{
		Title:     "📊 Linear Daily Digest",
		Color:     ColorBlue,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Footer: &DiscordFooter{
			Text: fmt.Sprintf("Total: %d open issues • Generated at", len(issues)),
		},
		Fields: []DiscordField{},
	}

	// Add summary description
	summaryParts := []string{fmt.Sprintf("**%d** open issues across your workspace", len(issues))}
	if len(priorityAlerts) > 0 {
		summaryParts = append(summaryParts, strings.Join(priorityAlerts, " | "))
	}
	mainEmbed.Description = strings.Join(summaryParts, "\n")

	// Add status breakdown
	mainEmbed.Fields = append(mainEmbed.Fields, DiscordField{
		Name:   "📋 By Status",
		Value:  strings.Join(statusLines, "\n"),
		Inline: true,
	})

	// Add assignee breakdown
	mainEmbed.Fields = append(mainEmbed.Fields, DiscordField{
		Name:   "👥 By Assignee",
		Value:  strings.Join(assigneeLines, "\n"),
		Inline: true,
	})

	embeds := []DiscordEmbed{mainEmbed}

	// Add urgent/high priority issues detail (if any)
	if urgentCount > 0 || highCount > 0 {
		var priorityIssues []string
		count := 0
		for _, issue := range issues {
			if issue.Priority <= 2 && count < 10 { // Top 10 urgent/high
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
			priorityEmbed := DiscordEmbed{
				Title:       "🚨 Priority Issues",
				Description: strings.Join(priorityIssues, "\n"),
				Color:       ColorRed,
			}
			embeds = append(embeds, priorityEmbed)
		}
	}

	// Add recent activity (issues updated today)
	today := time.Now().Truncate(24 * time.Hour)
	var recentIssues []string
	for _, issue := range issues {
		if issue.UpdatedAt.After(today) && len(recentIssues) < 5 {
			recentIssues = append(recentIssues, fmt.Sprintf("• [**%s**](%s) - %s",
				issue.Identifier, issue.URL, truncate(issue.Title, 50)))
		}
	}

	if len(recentIssues) > 0 {
		recentEmbed := DiscordEmbed{
			Title:       "🔄 Recently Updated",
			Description: strings.Join(recentIssues, "\n"),
			Color:       ColorYellow,
		}
		embeds = append(embeds, recentEmbed)
	}

	return sendToDiscord(&DiscordWebhook{
		Username:  "Linear Daily Digest",
		AvatarURL: "https://asset.brandfetch.io/ideiLNHwrW/id_xq4rBdb.png",
		Embeds:    embeds,
	})
}

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
