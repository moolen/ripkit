package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/moritz/mcp-toolkit/internal/audit"
)

// CheckPodIssues analyzes pod-related problems from audit logs
func (h *ToolHandlers) CheckPodIssues(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	startTime, endTime, err := parseTimeRange(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	namespace := request.GetString("namespace", "")

	// Query pod-related events
	events, err := h.auditClient.GetResourceTypeEvents(ctx, namespace, "pods", startTime, endTime)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query audit logs: %v", err)), nil
	}

	if len(events) == 0 {
		msg := "No pod events found in the specified time range"
		if namespace != "" {
			msg += fmt.Sprintf(" for namespace '%s'", namespace)
		}
		return mcp.NewToolResultText(msg + "."), nil
	}

	var results strings.Builder
	results.WriteString(fmt.Sprintf("Pod Issues Analysis (%s to %s)\n", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)))
	if namespace != "" {
		results.WriteString(fmt.Sprintf("Namespace: %s\n", namespace))
	}
	results.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Categorize pod issues
	crashLoopEvents := []audit.AuditEvent{}
	imagePullEvents := []audit.AuditEvent{}
	oomEvents := []audit.AuditEvent{}
	probeFailures := []audit.AuditEvent{}
	configIssues := []audit.AuditEvent{}
	replicaIssues := []audit.AuditEvent{}

	for _, event := range events {
		eventData, err := json.Marshal(event)
		if err != nil {
			continue
		}

		// 1: we have resource changes
		// 2: we have resource events

		combined := strings.ToLower(string(eventData))
		if strings.Contains(combined, "crashloopbackoff") {
			crashLoopEvents = append(crashLoopEvents, event)
		}
		if strings.Contains(combined, "imagepullbackoff") || strings.Contains(combined, "errimagepull") {
			imagePullEvents = append(imagePullEvents, event)
		}
		if strings.Contains(combined, "oomkilled") || strings.Contains(combined, "out of memory") {
			oomEvents = append(oomEvents, event)
		}
		if strings.Contains(combined, "liveness") || strings.Contains(combined, "readiness") ||
			strings.Contains(combined, "probe failed") {
			probeFailures = append(probeFailures, event)
		}
		if strings.Contains(combined, "configmap") || strings.Contains(combined, "secret") &&
			strings.Contains(combined, "not found") {
			configIssues = append(configIssues, event)
		}
		if strings.Contains(combined, "replica") &&
			(strings.Contains(combined, "insufficient") || strings.Contains(combined, "failed")) {
			replicaIssues = append(replicaIssues, event)
		}
	}

	// Report findings
	issueFound := false

	if len(crashLoopEvents) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("🔴 CrashLoopBackOff: %d events\n", len(crashLoopEvents)))
		for _, event := range crashLoopEvents[:min(5, len(crashLoopEvents))] {
			results.WriteString(fmt.Sprintf("  - %s: Pod %s/%s - %s\n",
				event.Timestamp.Format(time.RFC3339), event.Namespace, event.ResourceName, event.Message))
		}
		results.WriteString("\n")
	}

	if len(imagePullEvents) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("🔴 Image Pull Issues: %d events\n", len(imagePullEvents)))
		for _, event := range imagePullEvents[:min(5, len(imagePullEvents))] {
			results.WriteString(fmt.Sprintf("  - %s: Pod %s/%s - %s\n",
				event.Timestamp.Format(time.RFC3339), event.Namespace, event.ResourceName, event.Message))
		}
		results.WriteString("\n")
	}

	if len(oomEvents) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("🔴 OOMKilled: %d events\n", len(oomEvents)))
		for _, event := range oomEvents[:min(5, len(oomEvents))] {
			results.WriteString(fmt.Sprintf("  - %s: Pod %s/%s - %s\n",
				event.Timestamp.Format(time.RFC3339), event.Namespace, event.ResourceName, event.Message))
		}
		results.WriteString("\n")
	}

	if len(probeFailures) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("⚠️  Probe Failures: %d events\n", len(probeFailures)))
		for _, event := range probeFailures[:min(5, len(probeFailures))] {
			results.WriteString(fmt.Sprintf("  - %s: Pod %s/%s - %s\n",
				event.Timestamp.Format(time.RFC3339), event.Namespace, event.ResourceName, event.Message))
		}
		results.WriteString("\n")
	}

	if len(configIssues) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("⚠️  Config/Secret Issues: %d events\n", len(configIssues)))
		for _, event := range configIssues[:min(5, len(configIssues))] {
			results.WriteString(fmt.Sprintf("  - %s: Pod %s/%s - %s\n",
				event.Timestamp.Format(time.RFC3339), event.Namespace, event.ResourceName, event.Message))
		}
		results.WriteString("\n")
	}

	if len(replicaIssues) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("⚠️  Replica Scheduling Issues: %d events\n", len(replicaIssues)))
		for _, event := range replicaIssues[:min(3, len(replicaIssues))] {
			results.WriteString(fmt.Sprintf("  - %s: %s\n",
				event.Timestamp.Format(time.RFC3339), event.Message))
		}
		results.WriteString("\n")
	}

	if !issueFound {
		results.WriteString("✅ No critical pod issues detected.\n")
	}

	results.WriteString(fmt.Sprintf("\nTotal pod events analyzed: %d\n", len(events)))

	return mcp.NewToolResultText(results.String()), nil
}

// CheckVolumeIssues analyzes volume and storage-related problems
func (h *ToolHandlers) CheckVolumeIssues(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	startTime, endTime, err := parseTimeRange(request)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	namespace := request.GetString("namespace", "")

	var results strings.Builder
	results.WriteString(fmt.Sprintf("Volume Issues Analysis (%s to %s)\n", startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)))
	if namespace != "" {
		results.WriteString(fmt.Sprintf("Namespace: %s\n", namespace))
	}
	results.WriteString(strings.Repeat("=", 60) + "\n\n")

	// Query PVC events
	pvcEvents, err := h.auditClient.GetResourceTypeEvents(ctx, namespace, "persistentvolumeclaims", startTime, endTime)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query PVC events: %v", err)), nil
	}

	// Query PV events
	pvEvents, err := h.auditClient.GetResourceTypeEvents(ctx, "", "persistentvolumes", startTime, endTime)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to query PV events: %v", err)), nil
	}

	allEvents := append(pvcEvents, pvEvents...)

	if len(allEvents) == 0 {
		return mcp.NewToolResultText("No volume events found in the specified time range."), nil
	}

	// Categorize volume issues
	pendingPVC := []audit.AuditEvent{}
	bindingIssues := []audit.AuditEvent{}
	storageClassIssues := []audit.AuditEvent{}
	mountFailures := []audit.AuditEvent{}
	diskFullEvents := []audit.AuditEvent{}

	for _, event := range allEvents {
		msg := strings.ToLower(event.Message)
		annotations := strings.ToLower(fmt.Sprintf("%v", event.Annotations))
		combined := msg + " " + annotations

		if strings.Contains(combined, "pending") && event.ResourceType == "persistentvolumeclaims" {
			pendingPVC = append(pendingPVC, event)
		}
		if strings.Contains(combined, "binding") || strings.Contains(combined, "not bound") {
			bindingIssues = append(bindingIssues, event)
		}
		if strings.Contains(combined, "storageclass") &&
			(strings.Contains(combined, "error") || strings.Contains(combined, "failed")) {
			storageClassIssues = append(storageClassIssues, event)
		}
		if strings.Contains(combined, "mount") && strings.Contains(combined, "fail") {
			mountFailures = append(mountFailures, event)
		}
		if strings.Contains(combined, "disk full") || strings.Contains(combined, "no space left") {
			diskFullEvents = append(diskFullEvents, event)
		}
	}

	// Report findings
	issueFound := false

	if len(pendingPVC) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("⚠️  Pending PVCs: %d events\n", len(pendingPVC)))
		for _, event := range pendingPVC[:min(5, len(pendingPVC))] {
			results.WriteString(fmt.Sprintf("  - %s: PVC %s/%s - %s\n",
				event.Timestamp.Format(time.RFC3339), event.Namespace, event.ResourceName, event.Message))
		}
		results.WriteString("\n")
	}

	if len(bindingIssues) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("🔴 PV Binding Issues: %d events\n", len(bindingIssues)))
		for _, event := range bindingIssues[:min(5, len(bindingIssues))] {
			results.WriteString(fmt.Sprintf("  - %s: %s %s - %s\n",
				event.Timestamp.Format(time.RFC3339), event.ResourceType, event.ResourceName, event.Message))
		}
		results.WriteString("\n")
	}

	if len(storageClassIssues) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("🔴 StorageClass Errors: %d events\n", len(storageClassIssues)))
		for _, event := range storageClassIssues[:min(5, len(storageClassIssues))] {
			results.WriteString(fmt.Sprintf("  - %s: %s\n",
				event.Timestamp.Format(time.RFC3339), event.Message))
		}
		results.WriteString("\n")
	}

	if len(mountFailures) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("🔴 Volume Mount Failures: %d events\n", len(mountFailures)))
		for _, event := range mountFailures[:min(5, len(mountFailures))] {
			results.WriteString(fmt.Sprintf("  - %s: %s\n",
				event.Timestamp.Format(time.RFC3339), event.Message))
		}
		results.WriteString("\n")
	}

	if len(diskFullEvents) > 0 {
		issueFound = true
		results.WriteString(fmt.Sprintf("🔴 Disk Full Events: %d events\n", len(diskFullEvents)))
		for _, event := range diskFullEvents[:min(3, len(diskFullEvents))] {
			results.WriteString(fmt.Sprintf("  - %s: %s\n",
				event.Timestamp.Format(time.RFC3339), event.Message))
		}
		results.WriteString("\n")
	}

	if !issueFound {
		results.WriteString("✅ No volume issues detected.\n")
	}

	results.WriteString(fmt.Sprintf("\nTotal volume events analyzed: %d\n", len(allEvents)))

	return mcp.NewToolResultText(results.String()), nil
}
