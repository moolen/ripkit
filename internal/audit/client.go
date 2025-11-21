package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client provides access to Kubernetes audit logs via REST API
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new audit log API client
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// AuditEvent represents a Kubernetes audit log event
type AuditEvent struct {
	Timestamp      time.Time         `json:"timestamp"`
	Verb           string            `json:"verb"`
	User           string            `json:"user"`
	Namespace      string            `json:"namespace"`
	ResourceType   string            `json:"resourceType"`
	ResourceName   string            `json:"resourceName"`
	ResponseStatus int               `json:"responseStatus"`
	Message        string            `json:"message"`
	ObjectChanges  map[string]any    `json:"objectChanges,omitempty"`
	Annotations    map[string]string `json:"annotations,omitempty"`
	Stage          string            `json:"stage"`
	RequestURI     string            `json:"requestURI"`
	SourceIPs      []string          `json:"sourceIPs,omitempty"`
}

// QueryOptions defines parameters for querying audit events
type QueryOptions struct {
	StartTime    time.Time
	EndTime      time.Time
	Namespace    string
	ResourceType string
	ResourceName string
	Verb         string
	User         string
	Limit        int
}

// QueryEvents retrieves audit events based on the provided options
func (c *Client) QueryEvents(ctx context.Context, opts QueryOptions) ([]AuditEvent, error) {
	params := url.Values{}

	if !opts.StartTime.IsZero() {
		params.Add("start", opts.StartTime.Format(time.RFC3339))
	}
	if !opts.EndTime.IsZero() {
		params.Add("end", opts.EndTime.Format(time.RFC3339))
	}
	if opts.Namespace != "" {
		params.Add("namespace", opts.Namespace)
	}
	if opts.ResourceType != "" {
		params.Add("resourceType", opts.ResourceType)
	}
	if opts.ResourceName != "" {
		params.Add("resourceName", opts.ResourceName)
	}
	if opts.Verb != "" {
		params.Add("verb", opts.Verb)
	}
	if opts.User != "" {
		params.Add("user", opts.User)
	}
	if opts.Limit > 0 {
		params.Add("limit", fmt.Sprintf("%d", opts.Limit))
	}

	reqURL := fmt.Sprintf("%s/api/v1/events?%s", c.baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no audit data available for the specified time range")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var events []AuditEvent
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return events, nil
}

// GetNodeEvents retrieves audit events related to a specific node
func (c *Client) GetNodeEvents(ctx context.Context, nodeName string, startTime, endTime time.Time) ([]AuditEvent, error) {
	return c.QueryEvents(ctx, QueryOptions{
		StartTime:    startTime,
		EndTime:      endTime,
		ResourceType: "nodes",
		ResourceName: nodeName,
	})
}

// GetNamespaceEvents retrieves all audit events for a specific namespace
func (c *Client) GetNamespaceEvents(ctx context.Context, namespace string, startTime, endTime time.Time) ([]AuditEvent, error) {
	return c.QueryEvents(ctx, QueryOptions{
		StartTime: startTime,
		EndTime:   endTime,
		Namespace: namespace,
	})
}

// GetResourceTypeEvents retrieves audit events for a specific resource type
func (c *Client) GetResourceTypeEvents(ctx context.Context, namespace, resourceType string, startTime, endTime time.Time) ([]AuditEvent, error) {
	return c.QueryEvents(ctx, QueryOptions{
		StartTime:    startTime,
		EndTime:      endTime,
		Namespace:    namespace,
		ResourceType: resourceType,
	})
}

// GetRecentChanges retrieves create, update, patch, and delete events
func (c *Client) GetRecentChanges(ctx context.Context, startTime, endTime time.Time, resourceTypes []string) ([]AuditEvent, error) {
	verbs := []string{"create", "update", "patch", "delete"}

	// Build a single query with multiple verbs if API supports it, otherwise query separately
	opts := QueryOptions{
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     1000,
	}

	// For simplicity, query with verb filter - in production might use OR conditions
	var allEvents []AuditEvent
	for _, verb := range verbs {
		opts.Verb = verb
		events, err := c.QueryEvents(ctx, opts)
		if err != nil {
			// Don't fail on individual verb errors
			continue
		}

		// Filter by resource types if specified
		if len(resourceTypes) > 0 {
			filtered := make([]AuditEvent, 0)
			for _, event := range events {
				for _, rt := range resourceTypes {
					if strings.EqualFold(event.ResourceType, rt) {
						filtered = append(filtered, event)
						break
					}
				}
			}
			allEvents = append(allEvents, filtered...)
		} else {
			allEvents = append(allEvents, events...)
		}
	}

	return allEvents, nil
}
