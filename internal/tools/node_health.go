package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/moritz/mcp-toolkit/internal/audit"
)

// ToolHandlers contains all MCP tool handlers
type ToolHandlers struct {
	auditClient *audit.Client
}

// NewToolHandlers creates a new ToolHandlers instance
func NewToolHandlers(auditClient *audit.Client) *ToolHandlers {
	return &ToolHandlers{
		auditClient: auditClient,
	}
}

// parseTimeRange extracts start and end time from tool request
func parseTimeRange(request mcp.CallToolRequest) (time.Time, time.Time, error) {
	startStr, err := request.RequireString("start_time")
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("start_time is required (RFC3339 format)")
	}

	endStr, err := request.RequireString("end_time")
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("end_time is required (RFC3339 format)")
	}

	startTime, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start_time format: %w", err)
	}

	endTime, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid end_time format: %w", err)
	}

	if endTime.Before(startTime) {
		return time.Time{}, time.Time{}, fmt.Errorf("end_time must be after start_time")
	}

	return startTime, endTime, nil
}

// CheckNodeHealth checks for node-related issues in audit logs
func (h *ToolHandlers) CheckNodeHealth(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	startTime, endTime, err := parseTimeRange(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	// Query node-related events
	events, err := h.auditClient.GetResourceTypeEvents(ctx, "", "nodes", startTime, endTime)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query audit logs: %v", err)), nil
	}

	if len(events) == 0 {
		return mcp.NewToolResultText("No node events found in the specified time range."), nil
	}

	var results strings.Builder
	results.WriteString(fmt.Sprintf("Node Health Analysis (%s to %s)\n", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)))
	results.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Categorize node issues
	notReadyEvents := []audit.AuditEvent{}
	pressureEvents := []audit.AuditEvent{}
	networkEvents := []audit.AuditEvent{}
	kubeletEvents := []audit.AuditEvent{}

	for _, event := range events {
		msg := strings.ToLower(event.Message)
		annotations := strings.ToLower(fmt.Sprintf("%v", event.Annotations))

		if strings.Contains(msg, "notready") || strings.Contains(annotations, "notready") {
			notReadyEvents = append(notReadyEvents, event)
		}
		if strings.Contains(msg, "pressure") || strings.Contains(msg, "memorypressure") ||
			strings.Contains(msg, "diskpressure") {
			pressureEvents = append(pressureEvents, event)
		}
		if strings.Contains(msg, "network") && strings.Contains(msg, "unavailable") {
			networkEvents = append(networkEvents, event)
		}
		if strings.Contains(msg, "kubelet") {
			kubeletEvents = append(kubeletEvents, event)
		}
	}

	// Report findings
	if len(notReadyEvents) > 0 {
		results.WriteString(fmt.Sprintf("⚠️  NotReady Nodes: %d events\n", len(notReadyEvents)))
		for _, event := range notReadyEvents[:min(5, len(notReadyEvents))] {
			results.WriteString(fmt.Sprintf("  - %s: %s (Node: %s)\n",
				event.Timestamp.Format(time.RFC3339), event.Message, event.ResourceName))
		}
		results.WriteString("\n")
	}

	if len(pressureEvents) > 0 {
		results.WriteString(fmt.Sprintf("⚠️  Resource Pressure: %d events\n", len(pressureEvents)))
		for _, event := range pressureEvents[:min(5, len(pressureEvents))] {
			results.WriteString(fmt.Sprintf("  - %s: %s (Node: %s)\n",
				event.Timestamp.Format(time.RFC3339), event.Message, event.ResourceName))
		}
		results.WriteString("\n")
	}

	if len(networkEvents) > 0 {
		results.WriteString(fmt.Sprintf("⚠️  Network Issues: %d events\n", len(networkEvents)))
		for _, event := range networkEvents[:min(5, len(networkEvents))] {
			results.WriteString(fmt.Sprintf("  - %s: %s (Node: %s)\n",
				event.Timestamp.Format(time.RFC3339), event.Message, event.ResourceName))
		}
		results.WriteString("\n")
	}

	if len(kubeletEvents) > 0 {
		results.WriteString(fmt.Sprintf("ℹ️  Kubelet Events: %d events\n", len(kubeletEvents)))
		results.WriteString(fmt.Sprintf("  (Showing first 3 of %d)\n", len(kubeletEvents)))
		for _, event := range kubeletEvents[:min(3, len(kubeletEvents))] {
			results.WriteString(fmt.Sprintf("  - %s: %s\n",
				event.Timestamp.Format(time.RFC3339), event.Message))
		}
		results.WriteString("\n")
	}

	if len(notReadyEvents) == 0 && len(pressureEvents) == 0 && len(networkEvents) == 0 {
		results.WriteString("✅ No critical node health issues detected.\n")
	}

	results.WriteString(fmt.Sprintf("\nTotal node events analyzed: %d\n", len(events)))

	return mcp.NewToolResultText(results.String()), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
