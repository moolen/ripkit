package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/moritz/mcp-toolkit/internal/audit"
)

// AnalyzeRecentChanges shows recent modifications to Kubernetes resources
func (h *ToolHandlers) AnalyzeRecentChanges(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	startTime, endTime, err := parseTimeRange(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	resourceTypesStr := request.GetString("resource_types", "")
	var resourceTypes []string
	if resourceTypesStr != "" {
		resourceTypes = strings.Split(resourceTypesStr, ",")
		for i := range resourceTypes {
			resourceTypes[i] = strings.TrimSpace(resourceTypes[i])
		}
	}

	// Query for create, update, patch, delete events
	events, err := h.auditClient.GetRecentChanges(ctx, startTime, endTime, resourceTypes)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query audit logs: %v", err)), nil
	}

	if len(events) == 0 {
		msg := "No resource changes found in the specified time range"
		if len(resourceTypes) > 0 {
			msg += fmt.Sprintf(" for resource types: %s", strings.Join(resourceTypes, ", "))
		}
		return mcp.NewToolResultText(msg + "."), nil
	}

	var results strings.Builder
	results.WriteString(fmt.Sprintf("Recent Changes Analysis (%s to %s)\n", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)))
	if len(resourceTypes) > 0 {
		results.WriteString(fmt.Sprintf("Resource Types: %s\n", strings.Join(resourceTypes, ", ")))
	}
	results.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Group by resource type and verb
	changesByType := make(map[string]map[string]int)
	recentByType := make(map[string][]string)

	importantTypes := []string{"deployments", "configmaps", "secrets", "services", "ingresses", "daemonsets", "statefulsets"}

	for _, event := range events {
		rt := strings.ToLower(event.ResourceType)

		if changesByType[rt] == nil {
			changesByType[rt] = make(map[string]int)
		}
		changesByType[rt][event.Verb]++

		// Keep recent changes for important resource types
		isImportant := false
		for _, it := range importantTypes {
			if strings.Contains(rt, it) {
				isImportant = true
				break
			}
		}

		if isImportant && len(recentByType[rt]) < 5 {
			detail := fmt.Sprintf("  - %s: %s %s/%s by %s",
				event.Timestamp.Format("15:04:05"),
				event.Verb,
				event.Namespace,
				event.ResourceName,
				event.User)
			recentByType[rt] = append(recentByType[rt], detail)
		}
	}

	// Report deployments changes
	if changes, ok := changesByType["deployments"]; ok {
		results.WriteString("📦 Deployment Changes:\n")
		for verb, count := range changes {
			results.WriteString(fmt.Sprintf("  %s: %d\n", strings.ToUpper(verb), count))
		}
		if recent, ok := recentByType["deployments"]; ok {
			results.WriteString("  Recent changes:\n")
			for _, detail := range recent {
				results.WriteString(detail + "\n")
			}
		}
		results.WriteString("\n")
	}

	// Report config changes
	configChanges := 0
	if changes, ok := changesByType["configmaps"]; ok {
		for _, count := range changes {
			configChanges += count
		}
	}
	if changes, ok := changesByType["secrets"]; ok {
		for _, count := range changes {
			configChanges += count
		}
	}
	if configChanges > 0 {
		results.WriteString(fmt.Sprintf("⚙️  ConfigMap/Secret Changes: %d\n", configChanges))
		if recent, ok := recentByType["configmaps"]; ok && len(recent) > 0 {
			results.WriteString("  ConfigMaps:\n")
			for _, detail := range recent {
				results.WriteString(detail + "\n")
			}
		}
		if recent, ok := recentByType["secrets"]; ok && len(recent) > 0 {
			results.WriteString("  Secrets:\n")
			for _, detail := range recent {
				results.WriteString(detail + "\n")
			}
		}
		results.WriteString("\n")
	}

	// Report network changes
	networkChanges := 0
	if changes, ok := changesByType["services"]; ok {
		for _, count := range changes {
			networkChanges += count
		}
	}
	if changes, ok := changesByType["ingresses"]; ok {
		for _, count := range changes {
			networkChanges += count
		}
	}
	if changes, ok := changesByType["networkpolicies"]; ok {
		for _, count := range changes {
			networkChanges += count
		}
	}
	if networkChanges > 0 {
		results.WriteString(fmt.Sprintf("🌐 Network Changes: %d\n", networkChanges))
		results.WriteString("\n")
	}

	// Report other significant changes
	results.WriteString("Other Resource Changes:\n")
	for rt, changes := range changesByType {
		if rt != "deployments" && rt != "configmaps" && rt != "secrets" &&
			rt != "services" && rt != "ingresses" && rt != "networkpolicies" {
			totalChanges := 0
			for _, count := range changes {
				totalChanges += count
			}
			results.WriteString(fmt.Sprintf("  %s: %d changes\n", rt, totalChanges))
		}
	}

	results.WriteString(fmt.Sprintf("\nTotal change events: %d\n", len(events)))

	return mcp.NewToolResultText(results.String()), nil
}

// InvestigatePodStartup investigates why a pod won't start
func (h *ToolHandlers) InvestigatePodStartup(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	startTime, endTime, err := parseTimeRange(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	podName, err := request.RequireString("pod_name")
	if err != nil {
		return mcp.NewToolResultError("pod_name is required"), nil
	}

	namespace, err := request.RequireString("namespace")
	if err != nil {
		return mcp.NewToolResultError("namespace is required"), nil
	}

	// Query pod-specific events
	events, err := h.auditClient.QueryEvents(ctx, audit.QueryOptions{
		StartTime:    startTime,
		EndTime:      endTime,
		Namespace:    namespace,
		ResourceType: "pods",
		ResourceName: podName,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query audit logs: %v", err)), nil
	}

	if len(events) == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No events found for pod %s/%s in the specified time range.", namespace, podName)), nil
	}

	var results strings.Builder
	results.WriteString(fmt.Sprintf("Pod Startup Investigation: %s/%s\n", namespace, podName))
	results.WriteString(fmt.Sprintf("Time Range: %s to %s\n", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)))
	results.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Analyze different aspects
	imageIssues := []string{}
	secretIssues := []string{}
	volumeIssues := []string{}
	initContainerIssues := []string{}
	probeIssues := []string{}

	for _, event := range events {
		msg := strings.ToLower(event.Message)

		if strings.Contains(msg, "image") {
			if strings.Contains(msg, "pull") || strings.Contains(msg, "not found") ||
				strings.Contains(msg, "unauthorized") {
				imageIssues = append(imageIssues, fmt.Sprintf("[%s] %s", event.Timestamp.Format("15:04:05"), event.Message))
			}
		}
		if strings.Contains(msg, "secret") && strings.Contains(msg, "not found") {
			secretIssues = append(secretIssues, fmt.Sprintf("[%s] %s", event.Timestamp.Format("15:04:05"), event.Message))
		}
		if strings.Contains(msg, "volume") || strings.Contains(msg, "mount") {
			if strings.Contains(msg, "fail") || strings.Contains(msg, "error") {
				volumeIssues = append(volumeIssues, fmt.Sprintf("[%s] %s", event.Timestamp.Format("15:04:05"), event.Message))
			}
		}
		if strings.Contains(msg, "init") && strings.Contains(msg, "container") {
			initContainerIssues = append(initContainerIssues, fmt.Sprintf("[%s] %s", event.Timestamp.Format("15:04:05"), event.Message))
		}
		if strings.Contains(msg, "readiness") || strings.Contains(msg, "liveness") {
			probeIssues = append(probeIssues, fmt.Sprintf("[%s] %s", event.Timestamp.Format("15:04:05"), event.Message))
		}
	}

	// Report findings
	if len(imageIssues) > 0 {
		results.WriteString("🔍 Image Issues:\n")
		for _, issue := range imageIssues[:min(5, len(imageIssues))] {
			results.WriteString(fmt.Sprintf("  %s\n", issue))
		}
		results.WriteString("\n")
	}

	if len(secretIssues) > 0 {
		results.WriteString("🔍 Secret/Pull Secret Issues:\n")
		for _, issue := range secretIssues[:min(5, len(secretIssues))] {
			results.WriteString(fmt.Sprintf("  %s\n", issue))
		}
		results.WriteString("\n")
	}

	if len(volumeIssues) > 0 {
		results.WriteString("🔍 Volume Mount Issues:\n")
		for _, issue := range volumeIssues[:min(5, len(volumeIssues))] {
			results.WriteString(fmt.Sprintf("  %s\n", issue))
		}
		results.WriteString("\n")
	}

	if len(initContainerIssues) > 0 {
		results.WriteString("🔍 Init Container Issues:\n")
		for _, issue := range initContainerIssues[:min(5, len(initContainerIssues))] {
			results.WriteString(fmt.Sprintf("  %s\n", issue))
		}
		results.WriteString("\n")
	}

	if len(probeIssues) > 0 {
		results.WriteString("🔍 Probe Configuration:\n")
		for _, issue := range probeIssues[:min(3, len(probeIssues))] {
			results.WriteString(fmt.Sprintf("  %s\n", issue))
		}
		results.WriteString("\n")
	}

	if len(imageIssues) == 0 && len(secretIssues) == 0 && len(volumeIssues) == 0 &&
		len(initContainerIssues) == 0 && len(probeIssues) == 0 {
		results.WriteString("ℹ️  No obvious startup issues detected in audit logs.\n")
		results.WriteString("Recent events:\n")
		for _, event := range events[:min(5, len(events))] {
			results.WriteString(fmt.Sprintf("  [%s] %s: %s\n",
				event.Timestamp.Format("15:04:05"), event.Verb, event.Message))
		}
	}

	results.WriteString(fmt.Sprintf("\nTotal events analyzed: %d\n", len(events)))

	return mcp.NewToolResultText(results.String()), nil
}

// CheckResourceLimits analyzes resource limit related issues
func (h *ToolHandlers) CheckResourceLimits(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	startTime, endTime, err := parseTimeRange(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	namespace := request.GetString("namespace", "")

	// Query pod events for resource issues
	events, err := h.auditClient.GetResourceTypeEvents(ctx, namespace, "pods", startTime, endTime)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query audit logs: %v", err)), nil
	}

	// Also query node events for resource exhaustion
	nodeEvents, err := h.auditClient.GetResourceTypeEvents(ctx, "", "nodes", startTime, endTime)
	if err == nil {
		events = append(events, nodeEvents...)
	}

	if len(events) == 0 {
		return mcp.NewToolResultText("No resource limit events found in the specified time range."), nil
	}

	var results strings.Builder
	results.WriteString(fmt.Sprintf("Resource Limits Analysis (%s to %s)\n", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)))
	if namespace != "" {
		results.WriteString(fmt.Sprintf("Namespace: %s\n", namespace))
	}
	results.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Categorize resource issues
	cpuThrottling := []string{}
	oomKills := []string{}
	misconfigured := []string{}
	nodeExhaustion := []string{}

	for _, event := range events {
		msg := strings.ToLower(event.Message)

		if strings.Contains(msg, "cpu") && (strings.Contains(msg, "throttl") || strings.Contains(msg, "limit")) {
			cpuThrottling = append(cpuThrottling, fmt.Sprintf("[%s] %s/%s: %s",
				event.Timestamp.Format("15:04:05"), event.Namespace, event.ResourceName, event.Message))
		}
		if strings.Contains(msg, "oom") || strings.Contains(msg, "out of memory") {
			oomKills = append(oomKills, fmt.Sprintf("[%s] %s/%s: %s",
				event.Timestamp.Format("15:04:05"), event.Namespace, event.ResourceName, event.Message))
		}
		if strings.Contains(msg, "limit") && (strings.Contains(msg, "exceed") || strings.Contains(msg, "invalid")) {
			misconfigured = append(misconfigured, fmt.Sprintf("[%s] %s",
				event.Timestamp.Format("15:04:05"), event.Message))
		}
		if event.ResourceType == "nodes" &&
			(strings.Contains(msg, "insufficient") || strings.Contains(msg, "exhausted")) {
			nodeExhaustion = append(nodeExhaustion, fmt.Sprintf("[%s] Node %s: %s",
				event.Timestamp.Format("15:04:05"), event.ResourceName, event.Message))
		}
	}

	// Report findings
	issueFound := false

	if len(cpuThrottling) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("⚠️  CPU Throttling: %d events\n", len(cpuThrottling)))
		for _, issue := range cpuThrottling[:min(5, len(cpuThrottling))] {
			results.WriteString(fmt.Sprintf("  %s\n", issue))
		}
		results.WriteString("\n")
	}

	if len(oomKills) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("🔴 OOM Kills: %d events\n", len(oomKills)))
		for _, issue := range oomKills[:min(5, len(oomKills))] {
			results.WriteString(fmt.Sprintf("  %s\n", issue))
		}
		results.WriteString("\n")
	}

	if len(misconfigured) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("⚠️  Misconfigured Limits: %d events\n", len(misconfigured)))
		for _, issue := range misconfigured[:min(5, len(misconfigured))] {
			results.WriteString(fmt.Sprintf("  %s\n", issue))
		}
		results.WriteString("\n")
	}

	if len(nodeExhaustion) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("🔴 Node Resource Exhaustion: %d events\n", len(nodeExhaustion)))
		for _, issue := range nodeExhaustion[:min(5, len(nodeExhaustion))] {
			results.WriteString(fmt.Sprintf("  %s\n", issue))
		}
		results.WriteString("\n")
	}

	if !issueFound {
		results.WriteString("✅ No resource limit issues detected.\n")
	}

	results.WriteString(fmt.Sprintf("\nTotal events analyzed: %d\n", len(events)))

	return mcp.NewToolResultText(results.String()), nil
}
