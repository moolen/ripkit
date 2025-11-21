package watchers

import (
	"context"
	"fmt"
	"strings"

	"github.com/moritz/mcp-toolkit/internal/watch/config"
	"github.com/moritz/mcp-toolkit/internal/watch/models"
	"github.com/moritz/mcp-toolkit/internal/watch/storage"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Manager manages all resource watchers
type Manager struct {
	mgr    manager.Manager
	store  *storage.Store
	config *config.Config
}

// NewManager creates a new watcher manager
func NewManager(mgr manager.Manager, store *storage.Store, cfg *config.Config) *Manager {
	return &Manager{
		mgr:    mgr,
		store:  store,
		config: cfg,
	}
}

// Start initializes all watchers based on configuration
func (m *Manager) Start(ctx context.Context) error {
	// Register watchers for configured resources
	for _, resource := range m.config.Resources {
		if err := m.addWatcher(ctx, resource); err != nil {
			return fmt.Errorf("failed to add watcher for %s: %w", resource.Kind, err)
		}
	}

	// Discover and watch CRDs if enabled
	if m.config.DiscoverCRDs {
		if err := m.discoverCRDs(ctx); err != nil {
			// Log error but don't fail - CRDs might not be available
			fmt.Printf("Warning: failed to discover CRDs: %v\n", err)
		}
	}

	return nil
}

// addWatcher adds a watcher for a specific resource type
func (m *Manager) addWatcher(ctx context.Context, resource config.ResourceWatch) error {
	gvk := schema.GroupVersionKind{
		Group:   resource.Group,
		Version: resource.Version,
		Kind:    resource.Kind,
	}

	// Create an unstructured object for this resource type
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	// Get or create an informer for this resource type
	informer, err := m.mgr.GetCache().GetInformer(ctx, obj)
	if err != nil {
		return fmt.Errorf("failed to get informer: %w", err)
	}

	// Add event handlers
	_, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			m.handleAdd(obj)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			m.handleUpdate(oldObj, newObj)
		},
		DeleteFunc: func(obj interface{}) {
			m.handleDelete(obj)
		},
	})

	if err != nil {
		return fmt.Errorf("failed to add event handler: %w", err)
	}

	fmt.Printf("Started watching %s/%s (%s)\n", resource.Group, resource.Version, resource.Kind)
	return nil
}

// handleAdd handles object creation events
func (m *Manager) handleAdd(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		fmt.Printf("Warning: received non-unstructured object in Add event\n")
		return
	}

	event, err := models.TransformWatchEvent(u, models.EventTypeAdded)
	if err != nil {
		fmt.Printf("Error transforming Add event for %s/%s: %v\n", u.GetNamespace(), u.GetName(), err)
		return
	}

	if err := m.store.StoreEvent(context.Background(), event, u); err != nil {
		fmt.Printf("Error storing Add event for %s/%s: %v\n", u.GetNamespace(), u.GetName(), err)
	}
}

// handleUpdate handles object modification events
func (m *Manager) handleUpdate(oldObj, newObj interface{}) {
	u, ok := newObj.(*unstructured.Unstructured)
	if !ok {
		fmt.Printf("Warning: received non-unstructured object in Update event\n")
		return
	}

	event, err := models.TransformWatchEvent(u, models.EventTypeModified)
	if err != nil {
		fmt.Printf("Error transforming Update event for %s/%s: %v\n", u.GetNamespace(), u.GetName(), err)
		return
	}

	if err := m.store.StoreEvent(context.Background(), event, u); err != nil {
		fmt.Printf("Error storing Update event for %s/%s: %v\n", u.GetNamespace(), u.GetName(), err)
	}
}

// handleDelete handles object deletion events
func (m *Manager) handleDelete(obj interface{}) {
	u, ok := obj.(*unstructured.Unstructured)
	if !ok {
		fmt.Printf("Warning: received non-unstructured object in Delete event\n")
		return
	}

	event, err := models.TransformWatchEvent(u, models.EventTypeDeleted)
	if err != nil {
		fmt.Printf("Error transforming Delete event for %s/%s: %v\n", u.GetNamespace(), u.GetName(), err)
		return
	}

	if err := m.store.StoreEvent(context.Background(), event, u); err != nil {
		fmt.Printf("Error storing Delete event for %s/%s: %v\n", u.GetNamespace(), u.GetName(), err)
	}
}

// discoverCRDs discovers installed CRDs and adds watchers for them
func (m *Manager) discoverCRDs(ctx context.Context) error {
	// Create a direct client to list CRDs
	c := m.mgr.GetClient()

	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := c.List(ctx, crdList); err != nil {
		return fmt.Errorf("failed to list CRDs: %w", err)
	}

	for _, crd := range crdList.Items {
		// Skip if already in configured resources
		if m.isResourceConfigured(crd.Spec.Group, crd.Spec.Names.Kind) {
			continue
		}

		// Add watchers for each served version
		for _, version := range crd.Spec.Versions {
			if !version.Served {
				continue
			}

			resource := config.ResourceWatch{
				Group:      crd.Spec.Group,
				Version:    version.Name,
				Kind:       crd.Spec.Names.Kind,
				Plural:     crd.Spec.Names.Plural,
				Namespaced: crd.Spec.Scope == apiextensionsv1.NamespaceScoped,
			}

			if err := m.addWatcher(ctx, resource); err != nil {
				fmt.Printf("Warning: failed to watch CRD %s: %v\n", crd.Name, err)
				continue
			}
		}
	}

	// Also watch for new CRDs being created
	if err := m.watchCRDChanges(ctx); err != nil {
		fmt.Printf("Warning: failed to watch CRD changes: %v\n", err)
	}

	return nil
}

// isResourceConfigured checks if a resource is already in the configuration
func (m *Manager) isResourceConfigured(group, kind string) bool {
	for _, resource := range m.config.Resources {
		if resource.Group == group && resource.Kind == kind {
			return true
		}
	}
	return false
}

// watchCRDChanges watches for CRD creation/updates and adds watchers dynamically
func (m *Manager) watchCRDChanges(ctx context.Context) error {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	informer, err := m.mgr.GetCache().GetInformer(ctx, crd)
	if err != nil {
		return fmt.Errorf("failed to get CRD informer: %w", err)
	}

	_, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			crd, ok := obj.(*apiextensionsv1.CustomResourceDefinition)
			if !ok {
				return
			}

			// Add watchers for this new CRD
			for _, version := range crd.Spec.Versions {
				if !version.Served {
					continue
				}

				resource := config.ResourceWatch{
					Group:      crd.Spec.Group,
					Version:    version.Name,
					Kind:       crd.Spec.Names.Kind,
					Plural:     crd.Spec.Names.Plural,
					Namespaced: crd.Spec.Scope == apiextensionsv1.NamespaceScoped,
				}

				if err := m.addWatcher(context.Background(), resource); err != nil {
					fmt.Printf("Warning: failed to watch new CRD %s: %v\n", crd.Name, err)
				}
			}
		},
	})

	return err
}

// KindToResourceType converts a Kind to a resource type (plural lowercase)
func KindToResourceType(kind string) string {
	lower := strings.ToLower(kind)
	if strings.HasSuffix(lower, "s") {
		return lower + "es"
	}
	if strings.HasSuffix(lower, "y") {
		return strings.TrimSuffix(lower, "y") + "ies"
	}
	return lower + "s"
}
