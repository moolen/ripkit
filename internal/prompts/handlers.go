package prompts

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// PromptHandlers contains all MCP prompt handlers
type PromptHandlers struct{}

// NewPromptHandlers creates a new PromptHandlers instance
func NewPromptHandlers() *PromptHandlers {
	return &PromptHandlers{}
}

// InvestigatePodFailure guides investigation of pod failures
func (h *PromptHandlers) InvestigatePodFailure(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	podName := request.Params.Arguments["pod_name"]
	namespace := request.Params.Arguments["namespace"]
	timeWindow := request.Params.Arguments["time_window"]

	if timeWindow == "" {
		timeWindow = "1 hour"
	}

	prompt := fmt.Sprintf(`I need help investigating why pod "%s" in namespace "%s" is failing.

Investigation Steps:

1. **Check Pod Events** - Use the investigate_pod_startup tool with:
   - pod_name: %s
   - namespace: %s
   - time_window: last %s
   This will show image pull issues, mount failures, init container problems, etc.

2. **Check for Recent Changes** - Use analyze_recent_changes to see if any:
   - Deployments were updated
   - ConfigMaps or Secrets were modified
   - Network policies changed
   Focus on the last %s in namespace %s

3. **Check Resource Limits** - Use check_resource_limits to identify:
   - OOMKilled events
   - CPU throttling
   - Memory pressure
   Check namespace %s for the last %s

4. **Check Node Health** - If the pod can't be scheduled:
   - Use check_node_health to find node issues
   - Look for NotReady nodes, disk pressure, network problems

5. **Review Audit Logs Directly** - Access the resource:
   - audit://events/%s/pods for all pod events in the namespace
   - Look for patterns or recurring failures

Common Issues to Look For:
- Image pull errors (check image name, registry access, pull secrets)
- Missing ConfigMaps or Secrets
- Volume mount failures
- Init container failures
- Incorrect resource limits
- Node scheduling constraints
- Failed readiness/liveness probes

Please run the diagnostic tools and share the findings.`,
		podName, namespace, podName, namespace, timeWindow, timeWindow, namespace, namespace, timeWindow, namespace)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Investigation guide for pod %s/%s failure", namespace, podName),
		Messages: []mcp.PromptMessage{
			{
				Role:    mcp.RoleUser,
				Content: mcp.NewTextContent(prompt),
			},
		},
	}, nil
}

// DiagnoseClusterHealth guides overall cluster health diagnosis
func (h *PromptHandlers) DiagnoseClusterHealth(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	timeWindow := request.Params.Arguments["time_window"]
	focusArea := request.Params.Arguments["focus_area"]

	if timeWindow == "" {
		timeWindow = "24 hours"
	}
	if focusArea == "" {
		focusArea = "all"
	}

	prompt := fmt.Sprintf(`I need to diagnose the overall health of the Kubernetes cluster.

Time Window: Last %s
Focus Area: %s

Diagnostic Workflow:

1. **Node Health Check**
   - Run check_node_health for the last %s
   - Look for: NotReady nodes, memory/disk pressure, network issues, kubelet failures
   - Critical issues require immediate attention

2. **Pod Issues Analysis**
   - Run check_pod_issues across all namespaces
   - Identify: CrashLoopBackOff, ImagePullBackOff, OOMKilled pods
   - Check for probe failures and scheduling problems

3. **Volume Status**
   - Run check_volume_issues
   - Find: Pending PVCs, binding failures, StorageClass errors
   - Check for disk full events on nodes

4. **Recent Changes Review**
   - Run analyze_recent_changes for the last %s
   - Focus on: Deployments, ConfigMaps, Secrets, Network policies
   - Correlate changes with issues

5. **Resource Limits Analysis**
   - Run check_resource_limits
   - Identify: CPU throttling, OOM kills, node resource exhaustion
   - Find misconfigured resource requests/limits

Focus Areas:
- "nodes" - Deep dive into node health and capacity
- "pods" - Focus on pod-level issues and failures
- "storage" - Investigate volume and PVC problems
- "network" - Check service and ingress configurations
- "all" - Comprehensive cluster health check

After running diagnostics, prioritize issues by:
1. Critical (cluster-wide failures, multiple node issues)
2. High (service disruptions, pod failures)
3. Medium (performance degradation, warnings)
4. Low (informational events)

Please execute the relevant diagnostic tools and provide a summary of findings.`, timeWindow, focusArea, timeWindow, timeWindow)

	return &mcp.GetPromptResult{
		Description: "Comprehensive cluster health diagnosis guide",
		Messages: []mcp.PromptMessage{
			{
				Role:    mcp.RoleUser,
				Content: mcp.NewTextContent(prompt),
			},
		},
	}, nil
}

// AnalyzeDeploymentRollout guides deployment rollout investigation
func (h *PromptHandlers) AnalyzeDeploymentRollout(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	deploymentName := request.Params.Arguments["deployment_name"]
	namespace := request.Params.Arguments["namespace"]
	timeWindow := request.Params.Arguments["time_window"]

	if timeWindow == "" {
		timeWindow = "2 hours"
	}

	prompt := fmt.Sprintf(`I need to analyze a deployment rollout for "%s" in namespace "%s".

Investigation Steps:

1. **Review Recent Changes**
   - Run analyze_recent_changes with:
     - time_window: last %s
     - resource_types: "deployments,replicasets"
   - Look for the deployment update events
   - Check what changed (image, replicas, config references)

2. **Check Pod Issues**
   - Run check_pod_issues for namespace %s
   - Focus on new pods created by the deployment
   - Look for: CrashLoopBackOff, ImagePullBackOff, startup failures

3. **Investigate Individual Pod Failures**
   - Identify failing pod names from step 2
   - Use investigate_pod_startup for each failing pod
   - Check: Image availability, environment variables, volume mounts

4. **Check Resource Limits**
   - Run check_resource_limits for namespace %s
   - See if new pods are being OOMKilled
   - Check if CPU limits are causing throttling

5. **Review Rollout Progress**
   - Access audit://changes/%s for detailed change log
   - Look for:
     - Progressive vs stuck rollout
     - Healthy vs unhealthy replica counts
     - Rollback events

Common Rollout Issues:
- **Failed Image Pull**: Wrong image tag, registry issues, missing pull secrets
- **Configuration Errors**: Invalid ConfigMap/Secret references, wrong env vars
- **Resource Constraints**: Insufficient node resources, quota limits
- **Probe Failures**: Readiness/liveness probes failing for new version
- **Breaking Changes**: New code incompatible with existing dependencies

Rollout Strategies:
- If <50%% pods healthy: Consider immediate rollback
- If probe failures: Review probe configuration in new deployment
- If OOMKilled: Increase memory limits/requests
- If ImagePullBackOff: Verify image registry and credentials

Please run the diagnostic tools and determine if rollback is needed.`,
		deploymentName, namespace, timeWindow, namespace, namespace, timeWindow)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Deployment rollout analysis for %s/%s", namespace, deploymentName),
		Messages: []mcp.PromptMessage{
			{
				Role:    mcp.RoleUser,
				Content: mcp.NewTextContent(prompt),
			},
		},
	}, nil
}

// TroubleshootVolumeIssues guides volume troubleshooting
func (h *PromptHandlers) TroubleshootVolumeIssues(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	pvcName := request.Params.Arguments["pvc_name"]
	namespace := request.Params.Arguments["namespace"]

	prompt := fmt.Sprintf(`I need to troubleshoot volume issues for PVC "%s" in namespace "%s".

Investigation Steps:

1. **Check Volume Status**
   - Run check_volume_issues for namespace %s
   - Look for:
     - PVC stuck in Pending state
     - PV binding failures
     - StorageClass errors
     - Mount failures

2. **Review PVC Events**
   - Access audit://events/%s/persistentvolumeclaims
   - Find events related to %s
   - Check for provisioning errors or binding issues

3. **Check Node Volume Mounts**
   - Run check_node_health
   - Look for volume mount failures on specific nodes
   - Check for disk full events

4. **Verify Pod Attachment**
   - Run check_pod_issues for namespace %s
   - Find pods trying to use this PVC
   - Check if pods are stuck in ContainerCreating state

5. **Review Recent Changes**
   - Run analyze_recent_changes
   - Check for StorageClass modifications
   - Look for PV/PVC deletions or updates

Common Volume Issues:

**PVC Stuck in Pending:**
- No available PVs matching the claim
- StorageClass provisioner not working
- Insufficient storage capacity
- Access mode mismatch

**Mount Failures:**
- Node permissions issues
- Volume already mounted elsewhere (for ReadWriteOnce)
- Filesystem corruption
- Network storage unreachable

**Binding Issues:**
- PV and PVC selectors don't match
- Capacity mismatch
- Access mode incompatibility
- StorageClass name mismatch

**Performance Issues:**
- Disk full on backing storage
- I/O throttling
- Network latency (for remote storage)

Resolution Steps:
1. Check StorageClass exists and is default if not specified
2. Verify PV availability and capacity
3. Ensure node has permissions to mount volume
4. Check if volume is already in use (RWO volumes)
5. Review storage backend logs if provisioning fails

Please run the diagnostic tools to identify the root cause.`, pvcName, namespace, namespace, namespace, pvcName, namespace)

	return &mcp.GetPromptResult{
		Description: fmt.Sprintf("Volume troubleshooting guide for PVC %s/%s", namespace, pvcName),
		Messages: []mcp.PromptMessage{
			{
				Role:    mcp.RoleUser,
				Content: mcp.NewTextContent(prompt),
			},
		},
	}, nil
}
