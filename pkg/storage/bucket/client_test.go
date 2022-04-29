// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/bucket/client_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package bucket

import (
	"context"
	"io"
	"sync"
	"testing"

	"github.com/grafana/dskit/flagext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v2"

	util_log "github.com/grafana/mimir/pkg/util/log"
)

const (
	configWithS3Backend = `
backend: s3
s3:
  endpoint:          localhost
  bucket_name:       test
  access_key_id:     xxx
  secret_access_key: yyy
  insecure:          true
`

	configWithGCSBackend = `
backend: gcs
gcs:
  bucket_name:     test
  service_account: |-
    {
      "type": "service_account",
      "project_id": "id",
      "private_key_id": "id",
      "private_key": "-----BEGIN PRIVATE KEY-----\nSOMETHING\n-----END PRIVATE KEY-----\n",
      "client_email": "test@test.com",
      "client_id": "12345",
      "auth_uri": "https://accounts.google.com/o/oauth2/auth",
      "token_uri": "https://oauth2.googleapis.com/token",
      "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
      "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/test%40test.com"
    }
`

	configWithUnknownBackend = `
backend: unknown
`
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		config      string
		expectedErr error
	}{
		"should create an S3 bucket": {
			config:      configWithS3Backend,
			expectedErr: nil,
		},
		"should create a GCS bucket": {
			config:      configWithGCSBackend,
			expectedErr: nil,
		},
		"should return error on unknown backend": {
			config:      configWithUnknownBackend,
			expectedErr: ErrUnsupportedStorageBackend,
		},
	}

	for testName, testData := range tests {
		testData := testData

		t.Run(testName, func(t *testing.T) {
			// Load config
			cfg := Config{}
			flagext.DefaultValues(&cfg)

			err := yaml.Unmarshal([]byte(testData.config), &cfg)
			require.NoError(t, err)

			// Instance a new bucket client from the config
			bucketClient, err := NewClient(context.Background(), cfg, "test", util_log.Logger, nil)
			require.Equal(t, testData.expectedErr, err)

			if testData.expectedErr == nil {
				require.NotNil(t, bucketClient)
				bucketClient.Close()
			} else {
				assert.Equal(t, nil, bucketClient)
			}
		})
	}
}

func TestClientMock_MockGet(t *testing.T) {
	expected := "body"

	m := ClientMock{}
	m.MockGet("test", expected, nil)

	// Run many goroutines all requesting the same mocked object and
	// ensure there's no race.
	wg := sync.WaitGroup{}
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			reader, err := m.Get(context.Background(), "test")
			require.NoError(t, err)

			actual, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, []byte(expected), actual)

			require.NoError(t, reader.Close())
		}()
	}

	wg.Wait()
}

func TestClient_ConfigValidation(t *testing.T) {
	testCases := []struct {
		name           string
		cfg            Config
		expectingError bool
	}{
		{
			name:           "valid storage_prefix",
			cfg:            Config{Backend: Filesystem, StoragePrefix: "hello-world!"},
			expectingError: false,
		},
		{
			name:           "invalid storage_prefix",
			cfg:            Config{Backend: Filesystem, StoragePrefix: "/hello-world!"},
			expectingError: true,
		},
		{
			name:           "storage_prefix that has some character strings that have a meaning in unix paths (..)",
			cfg:            Config{Backend: Filesystem, StoragePrefix: ".."},
			expectingError: true,
		},
		{
			name:           "storage_prefix that has some character strings that have a meaning in unix paths (.)",
			cfg:            Config{Backend: Filesystem, StoragePrefix: "."},
			expectingError: true,
		},
		{
			name:           "unsupported backend",
			cfg:            Config{Backend: "flash drive"},
			expectingError: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if actualErr := tc.cfg.Validate(); tc.expectingError {
				assert.Error(t, actualErr)
			} else {
				assert.NoError(t, actualErr)
			}
		})
	}
}
