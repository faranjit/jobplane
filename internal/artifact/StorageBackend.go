// Package artifact provides pluggable storage backends for execution artifacts.
//
// Artifacts are files produced by jobs during execution (reports, models, datasets, etc.)
// that need to be persisted and made downloadable after the job completes. The package
// defines a StorageBackend interface that abstracts the underlying storage mechanism,
// allowing seamless transitions between backends (e.g., local filesystem → S3/GCS)
// without modifying worker or controller logic.
//
// The upload flow uses a simulated presigned URL pattern:
//  1. Worker requests authorization from the controller.
//  2. Controller generates a backend-specific upload URL (HMAC-signed for local, presigned for S3).
//  3. Worker performs a raw HTTP PUT to the returned URL.
//  4. Worker confirms the upload with the controller.
//
// This ensures the worker has exactly one code path regardless of the storage backend.
package artifact

import (
	"context"
	"jobplane/internal/config"

	"github.com/google/uuid"
)

// StorageBackend defines the contract for artifact storage backends.
// Each implementation generates backend-specific URLs for uploading and downloading artifacts.
// The controller delegates URL generation to the active backend, keeping handlers backend-agnostic.
//
// Implementations:
//   - LocalStorage: generates HMAC-signed controller URLs for local filesystem storage.
//   - S3 (future): generates AWS presigned PUT/GET URLs for direct S3 access.
type StorageBackend interface {
	// GenerateUploadURL creates a destination URL where the worker can upload artifact bytes.
	// For local storage, this returns a signed controller route.
	// For S3, this would return a presigned AWS PUT URL.
	GenerateUploadURL(ctx context.Context, meta ArtifactMeta) (UploadInstructions, error)

	// GenerateDownloadURL creates a URL from which clients can download an artifact.
	// For local storage, this returns a signed controller download route.
	// For S3, this would return a presigned GET URL.
	GenerateDownloadURL(ctx context.Context, storagePath string) (string, error)
}

// NewStorageBackend creates a StorageBackend based on the configured backend type.
// Currently supports "local" for filesystem storage. Returns nil for unsupported backends.
func NewStorageBackend(config *config.Config) StorageBackend {
	switch config.ArtifactStorageBackend {
	case "local":
		return NewLocalStorage(config.ArtifactStoragePath, []byte(config.ArtifactHMACSecret), config.ControllerURL, config.ArtifactMaxSizeBytes)
	default: // TODO: add other types when available
		return nil
	}
}

// ArtifactMeta holds the metadata needed to generate upload/download URLs.
// This is passed from the controller handler to the storage backend during the authorize step.
type ArtifactMeta struct {
	ArtifactID  uuid.UUID // Unique identifier for this artifact record
	ExecutionID uuid.UUID // The execution that produced this artifact
	TenantID    uuid.UUID // Tenant that owns the execution
	Filename    string    // Original filename of the artifact
	ContentType string    // MIME type (e.g., "application/octet-stream", "text/csv")
	SizeBytes   int64     // Declared size of the artifact in bytes
}

// UploadInstructions tells the worker how to upload the artifact bytes.
// The worker performs the HTTP request exactly as specified, without knowing the backend.
type UploadInstructions struct {
	URL       string `json:"upload_url"` // The URL to upload the artifact to
	Method    string `json:"method"`     // HTTP method to use (typically "PUT")
	ExpiresIn int64  `json:"expires_in"` // Seconds until the upload URL expires
}
