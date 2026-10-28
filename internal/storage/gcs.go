package storage

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"

	"cloud.google.com/go/storage"
	"github.com/charliek/envsecrets/internal/domain"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// Compile-time assertion that GCSStorage implements Storage
var _ Storage = (*GCSStorage)(nil)

// GCSStorage implements Storage using Google Cloud Storage
type GCSStorage struct {
	client *storage.Client
	bucket string
}

// validateGCSCredentials validates that the decoded credentials are valid JSON
// with the expected structure for a GCS service account
func validateGCSCredentials(decoded []byte) error {
	var creds map[string]interface{}
	if err := json.Unmarshal(decoded, &creds); err != nil {
		return domain.Errorf(domain.ErrGCSError, "credentials are not valid JSON: %v", err)
	}

	// Check for expected fields in a service account credential
	// At minimum, we expect "type" to be present
	credType, ok := creds["type"].(string)
	if !ok {
		return domain.Errorf(domain.ErrGCSError, "credentials missing 'type' field")
	}

	// Validate credential type
	validTypes := map[string]bool{
		"service_account":              true,
		"authorized_user":              true,
		"external_account":             true,
		"impersonated_service_account": true,
	}
	if !validTypes[credType] {
		return domain.Errorf(domain.ErrGCSError, "unsupported credential type: %s", credType)
	}

	return nil
}

// NewGCSStorage creates a new GCS storage client
func NewGCSStorage(ctx context.Context, bucket string, credentials string) (*GCSStorage, error) {
	var opts []option.ClientOption

	if credentials != "" {
		// Decode base64 credentials
		decoded, err := base64.StdEncoding.DecodeString(credentials)
		if err != nil {
			return nil, domain.Errorf(domain.ErrGCSError, "failed to decode credentials: %v", err)
		}

		// Validate JSON structure before passing to GCS client
		if err := validateGCSCredentials(decoded); err != nil {
			return nil, err
		}

		opts = append(opts, option.WithCredentialsJSON(decoded))
	}

	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, domain.Errorf(domain.ErrGCSError, "failed to create GCS client: %v", err)
	}

	return &GCSStorage{
		client: client,
		bucket: bucket,
	}, nil
}

// Upload implements Storage.Upload
func (s *GCSStorage) Upload(ctx context.Context, path string, r io.Reader) error {
	obj := s.client.Bucket(s.bucket).Object(path)
	w := obj.NewWriter(ctx)

	if _, err := io.Copy(w, r); err != nil {
		// On copy error, close the writer to abort the upload (ignore close error since we already have an error)
		_ = w.Close()
		return domain.Errorf(domain.ErrUploadFailed, "failed to write to GCS: %v", err)
	}

	if err := w.Close(); err != nil {
		return domain.Errorf(domain.ErrUploadFailed, "failed to close GCS writer: %v", err)
	}

	return nil
}

// Download implements Storage.Download
func (s *GCSStorage) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	obj := s.client.Bucket(s.bucket).Object(path)
	r, err := obj.NewReader(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return nil, domain.Errorf(domain.ErrFileNotFound, "object not found: %s", path)
		}
		return nil, domain.Errorf(domain.ErrDownloadFailed, "failed to read from GCS: %v", err)
	}
	return r, nil
}

// List implements Storage.List
func (s *GCSStorage) List(ctx context.Context, prefix string) ([]string, error) {
	var paths []string

	it := s.client.Bucket(s.bucket).Objects(ctx, &storage.Query{Prefix: prefix})
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, domain.Errorf(domain.ErrGCSError, "failed to list objects: %v", err)
		}
		paths = append(paths, attrs.Name)
	}

	return paths, nil
}

// Delete implements Storage.Delete
func (s *GCSStorage) Delete(ctx context.Context, path string) error {
	obj := s.client.Bucket(s.bucket).Object(path)
	if err := obj.Delete(ctx); err != nil {
		if err == storage.ErrObjectNotExist {
			return nil // Already deleted
		}
		return domain.Errorf(domain.ErrGCSError, "failed to delete object: %v", err)
	}
	return nil
}

// Exists implements Storage.Exists
func (s *GCSStorage) Exists(ctx context.Context, path string) (bool, error) {
	obj := s.client.Bucket(s.bucket).Object(path)
	_, err := obj.Attrs(ctx)
	if err == storage.ErrObjectNotExist {
		return false, nil
	}
	if err != nil {
		return false, domain.Errorf(domain.ErrGCSError, "failed to check object existence: %v", err)
	}
	return true, nil
}

// Close closes the GCS client
func (s *GCSStorage) Close() error {
	return s.client.Close()
}

// BucketExists checks if the configured bucket exists and is accessible
func (s *GCSStorage) BucketExists(ctx context.Context) (bool, error) {
	_, err := s.client.Bucket(s.bucket).Attrs(ctx)
	if err == storage.ErrBucketNotExist {
		return false, nil
	}
	if err != nil {
		return false, domain.Errorf(domain.ErrGCSError, "failed to check bucket: %v", err)
	}
	return true, nil
}
