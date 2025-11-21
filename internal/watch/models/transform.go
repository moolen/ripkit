package models

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	// SystemWatcherUser is the constant user for all watch events
	SystemWatcherUser = "system:k8s-watcher"

	// StageResponseComplete indicates the event was successfully recorded
	StageResponseComplete = "ResponseComplete"

	// ResponseStatusSuccess is the HTTP 200 status for successful watch events
	ResponseStatusSuccess = 200
)

// AuditEvent represents a Kubernetes audit log event
// This matches the structure expected by the MCP server client
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

// EventType represents the type of watch event
type EventType string

const (
	EventTypeAdded    EventType = "ADDED"
	EventTypeModified EventType = "MODIFIED"
	EventTypeDeleted  EventType = "DELETED"
)

// TransformWatchEvent converts an unstructured Kubernetes object and event type
// into an AuditEvent format suitable for storage and API responses
func TransformWatchEvent(obj *unstructured.Unstructured, eventType EventType) (*AuditEvent, error) {
	if obj == nil {
		return nil, fmt.Errorf("object cannot be nil")
	}

	// Map event type to verb
	verb := mapEventTypeToVerb(eventType)

	// Extract basic metadata
	namespace := obj.GetNamespace()
	name := obj.GetName()
	kind := obj.GetKind()
	resourceType := kindToResourceType(kind)

	// Clean the object by removing unnecessary fields
	cleanedObject := cleanObject(obj)

	// Build the audit event
	event := &AuditEvent{
		Timestamp:      time.Now(),
		Verb:           verb,
		User:           SystemWatcherUser,
		Namespace:      namespace,
		ResourceType:   resourceType,
		ResourceName:   name,
		ResponseStatus: ResponseStatusSuccess,
		Message:        formatMessage(verb, resourceType, namespace, name),
		ObjectChanges:  cleanedObject,
		Annotations:    obj.GetAnnotations(),
		Stage:          StageResponseComplete,
		RequestURI:     buildRequestURI(namespace, resourceType, name),
		SourceIPs:      []string{}, // Watch events don't have source IPs
	}

	return event, nil
}

// mapEventTypeToVerb converts watch event types to audit verbs
func mapEventTypeToVerb(eventType EventType) string {
	switch eventType {
	case EventTypeAdded:
		return "create"
	case EventTypeModified:
		return "update"
	case EventTypeDeleted:
		return "delete"
	default:
		return "unknown"
	}
}

// kindToResourceType converts a Kind (e.g., "Pod") to resource type (e.g., "pods")
// This is a simple pluralization - may need enhancement for irregular plurals
func kindToResourceType(kind string) string {
	lower := strings.ToLower(kind)

	// Handle special cases
	irregularPlurals := map[string]string{
		"endpoints":           "endpoints",
		"ingress":             "ingresses",
		"networkpolicy":       "networkpolicies",
		"poddisruptionbudget": "poddisruptionbudgets",
		"priorityclass":       "priorityclasses",
		"storageclass":        "storageclasses",
	}

	if plural, ok := irregularPlurals[lower]; ok {
		return plural
	}

	// Simple pluralization rules
	if strings.HasSuffix(lower, "s") {
		return lower + "es"
	}
	if strings.HasSuffix(lower, "y") {
		return strings.TrimSuffix(lower, "y") + "ies"
	}

	return lower + "s"
}

// cleanObject removes fields that are not needed for audit purposes
// This reduces storage size and removes noise
func cleanObject(obj *unstructured.Unstructured) map[string]any {
	// Deep copy the object to avoid modifying the original
	cleaned := obj.DeepCopy().Object

	// Remove metadata fields that are not useful for audit
	if metadata, ok := cleaned["metadata"].(map[string]any); ok {
		delete(metadata, "managedFields")
		delete(metadata, "resourceVersion")
		delete(metadata, "generation")
		delete(metadata, "selfLink")
		delete(metadata, "uid") // UID is used in keys, not needed in object
	}

	return cleaned
}

// formatMessage creates a human-readable message for the audit event
func formatMessage(verb, resourceType, namespace, name string) string {
	if namespace == "" {
		return fmt.Sprintf("%s %s %s", strings.Title(verb), resourceType, name)
	}
	return fmt.Sprintf("%s %s %s/%s", strings.Title(verb), resourceType, namespace, name)
}

// buildRequestURI constructs a Kubernetes API request URI
func buildRequestURI(namespace, resourceType, name string) string {
	if namespace == "" {
		// Cluster-scoped resource
		return fmt.Sprintf("/api/v1/%s/%s", resourceType, name)
	}
	// Namespaced resource
	return fmt.Sprintf("/api/v1/namespaces/%s/%s/%s", namespace, resourceType, name)
}

// ExtractInvolvedObject extracts the involvedObject reference from a Kubernetes Event
// Returns nil if the object is not an Event or doesn't have an involvedObject
func ExtractInvolvedObject(obj *unstructured.Unstructured) *ObjectReference {
	if obj.GetKind() != "Event" {
		return nil
	}

	involvedObj, found, err := unstructured.NestedMap(obj.Object, "involvedObject")
	if !found || err != nil {
		return nil
	}

	// Extract fields from involvedObject
	kind, _, _ := unstructured.NestedString(involvedObj, "kind")
	namespace, _, _ := unstructured.NestedString(involvedObj, "namespace")
	name, _, _ := unstructured.NestedString(involvedObj, "name")
	uid, _, _ := unstructured.NestedString(involvedObj, "uid")

	if kind == "" || name == "" {
		return nil
	}

	return &ObjectReference{
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
		UID:       uid,
	}
}

// ObjectReference represents a reference to a Kubernetes object
type ObjectReference struct {
	Kind      string
	Namespace string
	Name      string
	UID       string
}
