package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/moritz/mcp-toolkit/internal/audit"
)

// ResourceHandlers contains all MCP resource handlers
type ResourceHandlers struct {
	auditClient *audit.Client
}

// NewResourceHandlers creates a new ResourceHandlers instance
func NewResourceHandlers(auditClient *audit.Client) *ResourceHandlers {
	return &ResourceHandlers{
		auditClient: auditClient,
	}
}

// parseURIPath extracts components from a URI path
func parseURIPath(uri string) map[string]string {
	parts := make(map[string]string)

	// Remove scheme
	uri = strings.TrimPrefix(uri, "audit://")

	// Split by /
	segments := strings.Split(uri, "/")

	if len(segments) >= 2 {
		parts["type"] = segments[0]
		parts["param1"] = segments[1]
	}
	if len(segments) >= 3 {
		parts["param2"] = segments[2]
	}

	return parts
}

// HandleNamespaceEvents returns audit events for a specific namespace
func (h *ResourceHandlers) HandleNamespaceEvents(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	parts := parseURIPath(request.Params.URI)
	namespace := parts["param1"]

	if namespace == "" {
		return nil, fmt.Errorf("namespace not specified in URI")
	}

	// Default to last 24 hours
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)

	events, err := h.auditClient.GetNamespaceEvents(ctx, namespace, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch namespace events: %w", err)
	}

	data, err := json.MarshalIndent(map[string]any{
		"namespace": namespace,
		"timeRange": map[string]string{
			"start": startTime.Format(time.RFC3339),
			"end":   endTime.Format(time.RFC3339),
		},
		"eventCount": len(events),
		"events":     events,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal events: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

// HandleResourceTypeEvents returns audit events for a specific resource type in a namespace
func (h *ResourceHandlers) HandleResourceTypeEvents(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	parts := parseURIPath(request.Params.URI)
	namespace := parts["param1"]
	resourceType := parts["param2"]

	if namespace == "" || resourceType == "" {
		return nil, fmt.Errorf("namespace and resource type must be specified in URI")
	}

	// Default to last 24 hours
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)

	events, err := h.auditClient.GetResourceTypeEvents(ctx, namespace, resourceType, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch resource type events: %w", err)
	}

	data, err := json.MarshalIndent(map[string]any{
		"namespace":    namespace,
		"resourceType": resourceType,
		"timeRange": map[string]string{
			"start": startTime.Format(time.RFC3339),
			"end":   endTime.Format(time.RFC3339),
		},
		"eventCount": len(events),
		"events":     events,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal events: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

// HandleRecentChanges returns recent modification events
func (h *ResourceHandlers) HandleRecentChanges(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	parts := parseURIPath(request.Params.URI)
	timeRange := parts["param1"]

	var startTime time.Time
	endTime := time.Now()

	// Parse time range
	switch timeRange {
	case "1h":
		startTime = endTime.Add(-1 * time.Hour)
	case "24h":
		startTime = endTime.Add(-24 * time.Hour)
	case "7d":
		startTime = endTime.Add(-7 * 24 * time.Hour)
	default:
		startTime = endTime.Add(-24 * time.Hour)
	}

	events, err := h.auditClient.GetRecentChanges(ctx, startTime, endTime, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recent changes: %w", err)
	}

	data, err := json.MarshalIndent(map[string]any{
		"timeRange": map[string]string{
			"start": startTime.Format(time.RFC3339),
			"end":   endTime.Format(time.RFC3339),
		},
		"eventCount": len(events),
		"events":     events,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal events: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}

// HandleNodeEvents returns audit events for a specific node
func (h *ResourceHandlers) HandleNodeEvents(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	parts := parseURIPath(request.Params.URI)
	nodeName := parts["param1"]

	if nodeName == "" {
		return nil, fmt.Errorf("node name not specified in URI")
	}

	// Default to last 24 hours
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)

	events, err := h.auditClient.GetNodeEvents(ctx, nodeName, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch node events: %w", err)
	}

	data, err := json.MarshalIndent(map[string]any{
		"nodeName": nodeName,
		"timeRange": map[string]string{
			"start": startTime.Format(time.RFC3339),
			"end":   endTime.Format(time.RFC3339),
		},
		"eventCount": len(events),
		"events":     events,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal events: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      request.Params.URI,
			MIMEType: "application/json",
			Text:     string(data),
		},
	}, nil
}
