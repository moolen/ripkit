package main

import (
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/moritz/mcp-toolkit/internal/audit"
	"github.com/moritz/mcp-toolkit/internal/prompts"
	"github.com/moritz/mcp-toolkit/internal/resources"
	"github.com/moritz/mcp-toolkit/internal/tools"
)

func main() {
	// Get audit API URL from environment or use default
	auditAPIURL := os.Getenv("AUDIT_API_URL")
	if auditAPIURL == "" {
		auditAPIURL = "http://localhost:8080"
	}

	// Initialize audit client
	auditClient := audit.NewClient(auditAPIURL)

	// Initialize handlers
	toolHandlers := tools.NewToolHandlers(auditClient)
	resourceHandlers := resources.NewResourceHandlers(auditClient)
	promptHandlers := prompts.NewPromptHandlers()

	// Create MCP server with capabilities
	mcpServer := server.NewMCPServer(
		"k8s-audit-investigator",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, true),
		server.WithPromptCapabilities(true),
		server.WithInstructions("This server provides access to Kubernetes audit logs for incident investigation. Use the diagnostic tools to analyze cluster health, pod issues, volume problems, and recent changes. Prompt templates guide investigation workflows for common scenarios."),
	)

	// Register diagnostic tools
	mcpServer.AddTool(
		mcp.NewTool("check_node_health",
			mcp.WithDescription("Check for node health issues (NotReady, pressure, network, kubelet failures)"),
			mcp.WithString("start_time",
				mcp.Required(),
				mcp.Description("Start time in RFC3339 format (e.g., 2024-01-01T00:00:00Z)"),
			),
			mcp.WithString("end_time",
				mcp.Required(),
				mcp.Description("End time in RFC3339 format (e.g., 2024-01-01T23:59:59Z)"),
			),
		),
		toolHandlers.CheckNodeHealth,
	)

	mcpServer.AddTool(
		mcp.NewTool("check_pod_issues",
			mcp.WithDescription("Analyze pod problems (CrashLoopBackOff, ImagePullBackOff, OOMKilled, probe failures)"),
			mcp.WithString("start_time",
				mcp.Required(),
				mcp.Description("Start time in RFC3339 format"),
			),
			mcp.WithString("end_time",
				mcp.Required(),
				mcp.Description("End time in RFC3339 format"),
			),
			mcp.WithString("namespace",
				mcp.Description("Kubernetes namespace to filter by (optional)"),
			),
		),
		toolHandlers.CheckPodIssues,
	)

	mcpServer.AddTool(
		mcp.NewTool("check_volume_issues",
			mcp.WithDescription("Check volume and storage problems (PVC pending, binding failures, StorageClass errors)"),
			mcp.WithString("start_time",
				mcp.Required(),
				mcp.Description("Start time in RFC3339 format"),
			),
			mcp.WithString("end_time",
				mcp.Required(),
				mcp.Description("End time in RFC3339 format"),
			),
			mcp.WithString("namespace",
				mcp.Description("Kubernetes namespace to filter by (optional)"),
			),
		),
		toolHandlers.CheckVolumeIssues,
	)

	mcpServer.AddTool(
		mcp.NewTool("analyze_recent_changes",
			mcp.WithDescription("Show recent resource modifications (deployments, configs, secrets, network policies)"),
			mcp.WithString("start_time",
				mcp.Required(),
				mcp.Description("Start time in RFC3339 format"),
			),
			mcp.WithString("end_time",
				mcp.Required(),
				mcp.Description("End time in RFC3339 format"),
			),
			mcp.WithString("resource_types",
				mcp.Description("Comma-separated list of resource types to filter (e.g., 'deployments,configmaps')"),
			),
		),
		toolHandlers.AnalyzeRecentChanges,
	)

	mcpServer.AddTool(
		mcp.NewTool("investigate_pod_startup",
			mcp.WithDescription("Investigate why a specific pod won't start (image, secrets, volumes, init containers)"),
			mcp.WithString("start_time",
				mcp.Required(),
				mcp.Description("Start time in RFC3339 format"),
			),
			mcp.WithString("end_time",
				mcp.Required(),
				mcp.Description("End time in RFC3339 format"),
			),
			mcp.WithString("pod_name",
				mcp.Required(),
				mcp.Description("Name of the pod to investigate"),
			),
			mcp.WithString("namespace",
				mcp.Required(),
				mcp.Description("Namespace of the pod"),
			),
		),
		toolHandlers.InvestigatePodStartup,
	)

	mcpServer.AddTool(
		mcp.NewTool("check_resource_limits",
			mcp.WithDescription("Analyze resource limit issues (CPU throttling, OOM kills, node exhaustion)"),
			mcp.WithString("start_time",
				mcp.Required(),
				mcp.Description("Start time in RFC3339 format"),
			),
			mcp.WithString("end_time",
				mcp.Required(),
				mcp.Description("End time in RFC3339 format"),
			),
			mcp.WithString("namespace",
				mcp.Description("Kubernetes namespace to filter by (optional)"),
			),
		),
		toolHandlers.CheckResourceLimits,
	)

	// Register resources
	mcpServer.AddResource(
		mcp.NewResource(
			"audit://events/{namespace}",
			"Namespace Audit Events",
			mcp.WithResourceDescription("All audit events for a specific namespace (last 24 hours)"),
			mcp.WithMIMEType("application/json"),
		),
		resourceHandlers.HandleNamespaceEvents,
	)

	mcpServer.AddResource(
		mcp.NewResource(
			"audit://events/{namespace}/{resource-type}",
			"Resource Type Audit Events",
			mcp.WithResourceDescription("Audit events for a specific resource type in a namespace (last 24 hours)"),
			mcp.WithMIMEType("application/json"),
		),
		resourceHandlers.HandleResourceTypeEvents,
	)

	mcpServer.AddResource(
		mcp.NewResource(
			"audit://changes/{time-range}",
			"Recent Changes",
			mcp.WithResourceDescription("Recent resource modifications (time-range: 1h, 24h, 7d)"),
			mcp.WithMIMEType("application/json"),
		),
		resourceHandlers.HandleRecentChanges,
	)

	mcpServer.AddResource(
		mcp.NewResource(
			"audit://node-events/{node-name}",
			"Node Audit Events",
			mcp.WithResourceDescription("Audit events for a specific node (last 24 hours)"),
			mcp.WithMIMEType("application/json"),
		),
		resourceHandlers.HandleNodeEvents,
	)

	// Register investigation prompts
	mcpServer.AddPrompt(
		mcp.NewPrompt("investigate_pod_failure",
			mcp.WithPromptDescription("Step-by-step guide for investigating pod failures"),
			mcp.WithArgument("pod_name",
				mcp.ArgumentDescription("Name of the failing pod"),
				mcp.RequiredArgument(),
			),
			mcp.WithArgument("namespace",
				mcp.ArgumentDescription("Namespace of the pod"),
				mcp.RequiredArgument(),
			),
			mcp.WithArgument("time_window",
				mcp.ArgumentDescription("Time window to investigate (e.g., '1 hour', '2 hours')"),
			),
		),
		promptHandlers.InvestigatePodFailure,
	)

	mcpServer.AddPrompt(
		mcp.NewPrompt("diagnose_cluster_health",
			mcp.WithPromptDescription("Comprehensive cluster health diagnosis workflow"),
			mcp.WithArgument("time_window",
				mcp.ArgumentDescription("Time window for analysis (e.g., '24 hours', '7 days')"),
			),
			mcp.WithArgument("focus_area",
				mcp.ArgumentDescription("Area to focus on: nodes, pods, storage, network, or all"),
			),
		),
		promptHandlers.DiagnoseClusterHealth,
	)

	mcpServer.AddPrompt(
		mcp.NewPrompt("analyze_deployment_rollout",
			mcp.WithPromptDescription("Guide for analyzing deployment rollout issues"),
			mcp.WithArgument("deployment_name",
				mcp.ArgumentDescription("Name of the deployment"),
				mcp.RequiredArgument(),
			),
			mcp.WithArgument("namespace",
				mcp.ArgumentDescription("Namespace of the deployment"),
				mcp.RequiredArgument(),
			),
			mcp.WithArgument("time_window",
				mcp.ArgumentDescription("Time window since rollout started (e.g., '2 hours')"),
			),
		),
		promptHandlers.AnalyzeDeploymentRollout,
	)

	mcpServer.AddPrompt(
		mcp.NewPrompt("troubleshoot_volume_issues",
			mcp.WithPromptDescription("Guide for troubleshooting volume and PVC problems"),
			mcp.WithArgument("pvc_name",
				mcp.ArgumentDescription("Name of the PersistentVolumeClaim"),
				mcp.RequiredArgument(),
			),
			mcp.WithArgument("namespace",
				mcp.ArgumentDescription("Namespace of the PVC"),
				mcp.RequiredArgument(),
			),
		),
		promptHandlers.TroubleshootVolumeIssues,
	)

	// Start server with stdio transport
	if err := server.ServeStdio(mcpServer); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
