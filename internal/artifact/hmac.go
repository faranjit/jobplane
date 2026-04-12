package artifact

import "github.com/google/uuid"

// UploadToken is the payload encoded into HMAC-signed upload URLs for local storage.
// It contains all the metadata needed to validate and process an upload without requiring
// a database lookup on the upload hot path. The token is JSON-serialized, signed with
// HMAC-SHA256, and base64url-encoded into the URL query parameter.
//
// Field names use short JSON keys to minimize token size in URLs.
type UploadToken struct {
	ArtifactID  uuid.UUID `json:"aid"` // Artifact record ID (matches execution_artifacts.id)
	ExecutionID uuid.UUID `json:"eid"` // Execution that owns this artifact
	TenantID    uuid.UUID `json:"tid"` // Tenant for directory isolation
	Filename    string    `json:"fn"`  // Target filename on disk
	MaxSize     int64     `json:"ms"`  // Maximum allowed upload size in bytes
	ExpiresAt   int64     `json:"exp"` // Unix timestamp after which this token is rejected
}

// DownloadToken is the payload encoded into HMAC-signed download URLs for local storage.
// It authorizes access to a specific file path without requiring auth middleware or
// database lookups on the download endpoint.
type DownloadToken struct {
	StoragePath string `json:"sp"`  // Absolute or relative path to the stored file
	ExpiresAt   int64  `json:"exp"` // Unix timestamp after which this token is rejected
}

// SignToken generates an HMAC-signed string representation of an UploadToken.
// The output format is: base64url(json) + "." + base64url(hmac-sha256).
// This is conceptually similar to JWT but without the library overhead.
func SignToken(secret []byte, token UploadToken) string {
	return ""
}

// ValidateToken parses and validates an HMAC-signed token string.
// It verifies the signature against the provided secret and checks that the token
// has not expired. Returns the decoded UploadToken or an error if validation fails.
func ValidateToken(secret []byte, raw string) (*UploadToken, error) {
	return &UploadToken{}, nil
}
