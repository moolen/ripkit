package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/moritz/mcp-toolkit/internal/watch/models"
	"github.com/moritz/mcp-toolkit/internal/watch/storage"
)

// Server provides the REST API for querying watch events
type Server struct {
	store    *storage.Store
	maxLimit int
	router   *chi.Mux
}

// NewServer creates a new API server
func NewServer(store *storage.Store, maxLimit int) *Server {
	s := &Server{
		store:    store,
		maxLimit: maxLimit,
		router:   chi.NewRouter(),
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes() {
	s.router.Use(middleware.Logger)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.RequestID)

	s.router.Get("/api/v1/events", s.handleQueryEvents)
	s.router.Get("/api/v1/events/{namespace}/{resourceType}/{name}", s.handleObjectHistory)
	s.router.Get("/health", s.handleHealth)
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

// handleQueryEvents handles time-range and filtered queries
func (s *Server) handleQueryEvents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	opts := storage.QueryOptions{
		Namespace:    r.URL.Query().Get("namespace"),
		ResourceType: r.URL.Query().Get("resourceType"),
		ResourceName: r.URL.Query().Get("resourceName"),
		Verb:         r.URL.Query().Get("verb"),
		User:         r.URL.Query().Get("user"),
	}

	// Parse time range
	if startStr := r.URL.Query().Get("start"); startStr != "" {
		startTime, err := time.Parse(time.RFC3339, startStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid start time format: %v", err), http.StatusBadRequest)
			return
		}
		opts.StartTime = startTime
	}

	if endStr := r.URL.Query().Get("end"); endStr != "" {
		endTime, err := time.Parse(time.RFC3339, endStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid end time format: %v", err), http.StatusBadRequest)
			return
		}
		opts.EndTime = endTime
	}

	// Parse limit with max enforcement
	limit := s.maxLimit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid limit: %v", err), http.StatusBadRequest)
			return
		}
		if parsedLimit > 0 && parsedLimit < limit {
			limit = parsedLimit
		}
	}
	opts.Limit = limit

	// Query the store
	events, err := s.store.QueryEvents(ctx, opts)
	if err != nil {
		http.Error(w, fmt.Sprintf("Query failed: %v", err), http.StatusInternalServerError)
		return
	}

	// If no events found, return 404
	if len(events) == 0 {
		http.Error(w, "no audit data available for the specified time range", http.StatusNotFound)
		return
	}

	// Set pagination headers
	w.Header().Set("X-Total-Count", strconv.Itoa(len(events)))
	if len(events) >= limit {
		w.Header().Set("X-Has-More", "true")
		// Could add Link header with next page URL if implementing cursor pagination
	} else {
		w.Header().Set("X-Has-More", "false")
	}

	// Return events as JSON array (matching existing client expectations)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(events); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// ObjectEventsResponse contains both direct watch events and related Event objects
type ObjectEventsResponse struct {
	Namespace     string               `json:"namespace"`
	ResourceType  string               `json:"resourceType"`
	ResourceName  string               `json:"resourceName"`
	WatchEvents   []*models.AuditEvent `json:"watchEvents"`
	RelatedEvents []*models.AuditEvent `json:"relatedEvents"`
}

// handleObjectHistory returns all events for a specific object in two sections
func (s *Server) handleObjectHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	namespace := chi.URLParam(r, "namespace")
	resourceType := chi.URLParam(r, "resourceType")
	name := chi.URLParam(r, "name")

	// Validate parameters
	if namespace == "" || resourceType == "" || name == "" {
		http.Error(w, "namespace, resourceType, and name are required", http.StatusBadRequest)
		return
	}

	// Get direct watch events for this object
	watchEvents, err := s.store.GetObjectHistory(ctx, namespace, resourceType, name)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to query object history: %v", err), http.StatusInternalServerError)
		return
	}

	// Get related Event objects (where involvedObject points to this object)
	// Convert resourceType to Kind (pods -> Pod)
	kind := resourceTypeToKind(resourceType)
	relatedEvents, err := s.store.GetRelatedEvents(ctx, namespace, kind, name)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to query related events: %v", err), http.StatusInternalServerError)
		return
	}

	// Build response with two sections
	response := ObjectEventsResponse{
		Namespace:     namespace,
		ResourceType:  resourceType,
		ResourceName:  name,
		WatchEvents:   watchEvents,
		RelatedEvents: relatedEvents,
	}

	// Return 404 if no data found at all
	if len(watchEvents) == 0 && len(relatedEvents) == 0 {
		http.Error(w, "no events found for this object", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// handleHealth provides a health check endpoint
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

// resourceTypeToKind converts resource type (plural lowercase) to Kind (singular capitalized)
func resourceTypeToKind(resourceType string) string {
	// Handle special cases
	irregularSingulars := map[string]string{
		"endpoints":                 "Endpoints",
		"ingresses":                 "Ingress",
		"networkpolicies":           "NetworkPolicy",
		"poddisruptionbudgets":      "PodDisruptionBudget",
		"priorityclasses":           "PriorityClass",
		"storageclasses":            "StorageClass",
		"customresourcedefinitions": "CustomResourceDefinition",
	}

	if singular, ok := irregularSingulars[resourceType]; ok {
		return singular
	}

	// Simple singularization rules
	singular := resourceType
	if strings.HasSuffix(singular, "ies") {
		singular = strings.TrimSuffix(singular, "ies") + "y"
	} else if strings.HasSuffix(singular, "ses") {
		singular = strings.TrimSuffix(singular, "ses")
	} else if strings.HasSuffix(singular, "es") {
		singular = strings.TrimSuffix(singular, "es")
	} else if strings.HasSuffix(singular, "s") {
		singular = strings.TrimSuffix(singular, "s")
	}

	// Capitalize first letter
	if len(singular) > 0 {
		return strings.ToUpper(singular[:1]) + singular[1:]
	}

	return singular
}
