package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/moritz/mcp-toolkit/internal/watch/models"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Store manages BadgerDB storage for watch events
type Store struct {
	db            *badger.DB
	retentionDays int
}

// NewStore creates a new BadgerDB store
func NewStore(path string, retentionDays int) (*Store, error) {
	opts := badger.DefaultOptions(path)
	opts.SyncWrites = false // Async writes for better performance
	opts.NumVersionsToKeep = 1
	opts.ValueLogFileSize = 256 << 20 // 256 MB value log files
	opts.ValueLogMaxEntries = 500000

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to open BadgerDB: %w", err)
	}

	return &Store{
		db:            db,
		retentionDays: retentionDays,
	}, nil
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// StoreEvent stores an audit event with appropriate indexes
func (s *Store) StoreEvent(ctx context.Context, event *models.AuditEvent, obj *unstructured.Unstructured) error {
	// Serialize the event
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	ttl := time.Duration(s.retentionDays) * 24 * time.Hour
	expiresAt := uint64(time.Now().Add(ttl).Unix())
	uid := string(obj.GetUID())

	return s.db.Update(func(txn *badger.Txn) error {
		// Primary time-based index for time-range queries
		timeKey := fmt.Sprintf("events/%s/%s/%s/%s/%s",
			event.Timestamp.Format(time.RFC3339),
			event.Namespace,
			event.ResourceType,
			event.ResourceName,
			uid)

		if err := txn.SetEntry(&badger.Entry{
			Key:       []byte(timeKey),
			Value:     data,
			ExpiresAt: expiresAt,
		}); err != nil {
			return fmt.Errorf("failed to store time index: %w", err)
		}

		// Object-based index for object history queries
		objectKey := fmt.Sprintf("objects/%s/%s/%s/%s/%s",
			event.Namespace,
			event.ResourceType,
			event.ResourceName,
			event.Timestamp.Format(time.RFC3339),
			uid)

		if err := txn.SetEntry(&badger.Entry{
			Key:       []byte(objectKey),
			Value:     data,
			ExpiresAt: expiresAt,
		}); err != nil {
			return fmt.Errorf("failed to store object index: %w", err)
		}

		// Special handling for Event objects - create reference index
		if event.ResourceType == "events" {
			involvedObj := models.ExtractInvolvedObject(obj)
			if involvedObj != nil {
				refKey := fmt.Sprintf("eventRefs/%s/%s/%s/%s/%s",
					involvedObj.Namespace,
					involvedObj.Kind,
					involvedObj.Name,
					event.Timestamp.Format(time.RFC3339),
					uid)

				if err := txn.SetEntry(&badger.Entry{
					Key:       []byte(refKey),
					Value:     data,
					ExpiresAt: expiresAt,
				}); err != nil {
					return fmt.Errorf("failed to store event reference: %w", err)
				}
			}
		}

		return nil
	})
}

// QueryOptions defines parameters for querying events
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

// QueryEvents retrieves events based on query options
func (s *Store) QueryEvents(ctx context.Context, opts QueryOptions) ([]*models.AuditEvent, error) {
	var events []*models.AuditEvent
	count := 0
	limit := opts.Limit
	if limit <= 0 {
		limit = 1000 // Default max
	}

	err := s.db.View(func(txn *badger.Txn) error {
		iterOpts := badger.DefaultIteratorOptions
		iterOpts.PrefetchValues = true
		iterOpts.PrefetchSize = 100

		iter := txn.NewIterator(iterOpts)
		defer iter.Close()

		// Build prefix for time-based search
		prefix := "events/"
		if !opts.StartTime.IsZero() {
			prefix += opts.StartTime.Format(time.RFC3339)
		}

		for iter.Seek([]byte(prefix)); iter.ValidForPrefix([]byte("events/")); iter.Next() {
			if count >= limit {
				break
			}

			item := iter.Item()
			key := string(item.Key())

			// Parse key: events/{timestamp}/{namespace}/{resourceType}/{resourceName}/{uid}
			parts := strings.Split(key, "/")
			if len(parts) < 6 {
				continue
			}

			timestamp, err := time.Parse(time.RFC3339, parts[1])
			if err != nil {
				continue
			}

			// Filter by time range
			if !opts.EndTime.IsZero() && timestamp.After(opts.EndTime) {
				break // Keys are sorted by time, so we can stop
			}
			if !opts.StartTime.IsZero() && timestamp.Before(opts.StartTime) {
				continue
			}

			// Filter by namespace
			if opts.Namespace != "" && parts[2] != opts.Namespace {
				continue
			}

			// Filter by resource type
			if opts.ResourceType != "" && parts[3] != opts.ResourceType {
				continue
			}

			// Filter by resource name
			if opts.ResourceName != "" && parts[4] != opts.ResourceName {
				continue
			}

			// Get the event data
			err = item.Value(func(val []byte) error {
				var event models.AuditEvent
				if err := json.Unmarshal(val, &event); err != nil {
					return err
				}

				// Filter by verb
				if opts.Verb != "" && event.Verb != opts.Verb {
					return nil
				}

				// Filter by user
				if opts.User != "" && event.User != opts.User {
					return nil
				}

				events = append(events, &event)
				count++
				return nil
			})

			if err != nil {
				return err
			}
		}

		return nil
	})

	return events, err
}

// GetObjectHistory retrieves all events for a specific object
func (s *Store) GetObjectHistory(ctx context.Context, namespace, resourceType, name string) ([]*models.AuditEvent, error) {
	var events []*models.AuditEvent

	err := s.db.View(func(txn *badger.Txn) error {
		iterOpts := badger.DefaultIteratorOptions
		iterOpts.PrefetchValues = true

		iter := txn.NewIterator(iterOpts)
		defer iter.Close()

		// Build prefix for object-based search
		prefix := fmt.Sprintf("objects/%s/%s/%s/", namespace, resourceType, name)

		for iter.Seek([]byte(prefix)); iter.ValidForPrefix([]byte(prefix)); iter.Next() {
			item := iter.Item()

			err := item.Value(func(val []byte) error {
				var event models.AuditEvent
				if err := json.Unmarshal(val, &event); err != nil {
					return err
				}
				events = append(events, &event)
				return nil
			})

			if err != nil {
				return err
			}
		}

		return nil
	})

	return events, err
}

// GetRelatedEvents retrieves Event objects that reference a specific object
func (s *Store) GetRelatedEvents(ctx context.Context, namespace, kind, name string) ([]*models.AuditEvent, error) {
	var events []*models.AuditEvent

	err := s.db.View(func(txn *badger.Txn) error {
		iterOpts := badger.DefaultIteratorOptions
		iterOpts.PrefetchValues = true

		iter := txn.NewIterator(iterOpts)
		defer iter.Close()

		// Build prefix for event reference search
		prefix := fmt.Sprintf("eventRefs/%s/%s/%s/", namespace, kind, name)

		for iter.Seek([]byte(prefix)); iter.ValidForPrefix([]byte(prefix)); iter.Next() {
			item := iter.Item()

			err := item.Value(func(val []byte) error {
				var event models.AuditEvent
				if err := json.Unmarshal(val, &event); err != nil {
					return err
				}
				events = append(events, &event)
				return nil
			})

			if err != nil {
				return err
			}
		}

		return nil
	})

	return events, err
}

// RunGC runs BadgerDB garbage collection
func (s *Store) RunGC(ctx context.Context, discardRatio float64) error {
	return s.db.RunValueLogGC(discardRatio)
}

// StartGCRoutine starts a background goroutine for periodic GC
func (s *Store) StartGCRoutine(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := s.RunGC(ctx, 0.5) // Discard 50% stale data
			if err != nil && err != badger.ErrNoRewrite {
				// Log error but continue
				fmt.Printf("GC error: %v\n", err)
			}
		}
	}
}
