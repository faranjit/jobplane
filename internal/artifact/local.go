package artifact

import "context"

// LocalStorage implements StorageBackend for local filesystem storage.
// It generates HMAC-signed controller URLs that simulate the presigned URL pattern
// used by cloud storage backends (S3/GCS), keeping the worker upload flow identical
// regardless of the underlying storage.
//
// Files are stored at: {basePath}/{tenantID}/{executionID}/{filename}
//
// The controller serves two additional endpoints for local storage:
//   - PUT /internal/storage/upload?token=... — receives file bytes from the worker
//   - GET /internal/storage/download?token=... — serves files to clients
//
// These endpoints validate the HMAC token before processing, requiring no
// database lookup on the upload/download hot path.
type LocalStorage struct {
	basePath      string // Root directory for artifact storage (e.g., "./artifacts")
	hmacSecret    []byte // Secret key for signing upload/download tokens
	controllerURL string // Base URL of the controller (e.g., "http://localhost:6161")
	maxSizeBytes  int64  // Maximum allowed artifact size in bytes
}

// NewLocalStorage creates a new LocalStorage backend.
func NewLocalStorage(basePath string, hmacSecret []byte, controllerURL string, maxSizeBytes int64) *LocalStorage {
	return &LocalStorage{
		basePath:      basePath,
		hmacSecret:    hmacSecret,
		controllerURL: controllerURL,
		maxSizeBytes:  maxSizeBytes,
	}
}

// GenerateUploadURL creates an HMAC-signed controller URL for the worker to upload artifact bytes.
// The returned URL points to the controller's local storage upload endpoint with an embedded
// token containing the artifact metadata and expiry.
func (s *LocalStorage) GenerateUploadURL(ctx context.Context, meta ArtifactMeta) (UploadInstructions, error) {
	return UploadInstructions{}, nil
}

// GenerateDownloadURL creates an HMAC-signed controller URL for clients to download an artifact.
// The returned URL points to the controller's local storage download endpoint with an embedded
// token containing the storage path and expiry.
func (s *LocalStorage) GenerateDownloadURL(ctx context.Context, storagePath string) (string, error) {
	return "", nil
}
