// Copyright 2020 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secrets

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	secretmanager "cloud.google.com/go/secretmanager/apiv1beta1"
	"github.com/googleapis/gax-go"
	"golang.org/x/oauth2"
	"google.golang.org/api/option"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1beta1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// SecretManagerSource is a Source that uses Google Secret Manager.
//
// Construct it with NewSecretManagerSource.
type SecretManagerSource struct {
	client secretManagerClient // usually *secretmanager.Client

	secretURL string // e.g. "sm://<project>/<secret>""
	project   string // parsed from secretURL
	secret    string // parsed from secretURL

	mu       sync.Mutex
	versions [2]int64 // the latest and one previous (if enabled) version numbers
}

// secretManagerClient is subset of *secretmanager.Client we use.
//
// Mocked in tests.
type secretManagerClient interface {
	AccessSecretVersion(context.Context, *secretmanagerpb.AccessSecretVersionRequest, ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error)
	Close() error
}

// NewSecretManagerSource parses "sm://<project>/<name>" and sets up the source
// that fetches the latest two versions of this GSM secret as a single *Secret.
//
// Uses the given TokenSource for authentication.
func NewSecretManagerSource(ctx context.Context, secretURL string, ts oauth2.TokenSource) (*SecretManagerSource, error) {
	if !strings.HasPrefix(secretURL, "sm://") {
		return nil, errors.Reason("not a sm://... URL").Err()
	}
	chunks := strings.Split(strings.TrimPrefix(secretURL, "sm://"), "/")
	if len(chunks) != 2 {
		return nil, errors.Reason("sm://... URL should have form sm://<project>/<secret>").Err()
	}

	client, err := secretmanager.NewClient(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, errors.Annotate(err, "failed to setup Secret Manager client").Err()
	}

	return &SecretManagerSource{
		client:    client,
		secretURL: secretURL,
		project:   chunks[0],
		secret:    chunks[1],
	}, nil
}

// ReadSecret is part of Source interface.
func (s *SecretManagerSource) ReadSecret(ctx context.Context) (*Secret, error) {
	// Grab the latest version.
	latest, err := s.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", s.project, s.secret),
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to get the latest version of the secret").Err()
	}

	// The version name has format projects/.../secrets/.../versions/<number>.
	// We want to grab the version number to peek at the previous one (if any).
	idx := strings.LastIndex(latest.Name, "/")
	if idx == -1 {
		return nil, errors.Reason("unexpected version name format %q", latest.Name).Err()
	}
	version, err := strconv.ParseInt(latest.Name[idx+1:], 10, 64)
	if err != nil {
		return nil, errors.Reason("unexpected version name format %q", latest.Name).Err()
	}

	// If potentially have a previous version, try to grab it.
	var previous *secretmanagerpb.AccessSecretVersionResponse
	if version > 1 {
		previous, err = s.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
			Name: fmt.Sprintf("projects/%s/secrets/%s/versions/%d", s.project, s.secret, version-1),
		})
		switch status.Code(err) {
		case codes.OK:
			// Got it.
		case codes.FailedPrecondition:
			// The version is already disabled, this is fine.
		case codes.NotFound:
			// The version is already deleted, this is also fine.
		default:
			return nil, errors.Annotate(err, "failed to read the previous version of the secret").Err()
		}
	}

	versions := [2]int64{version, 0}
	if previous != nil {
		versions[1] = version - 1
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Log if any of the state has changed.
	if versions != s.versions {
		formatVer := func(v int64) string {
			if v == 0 {
				return "none"
			}
			return strconv.FormatInt(v, 10)
		}
		logging.Infof(ctx, "Active versions of the secret %s: latest=%s, previous=%s",
			s.secretURL,
			formatVer(versions[0]),
			formatVer(versions[1]),
		)
		s.versions = versions
	}

	// Return both versions as a single *Secret so tokens created using older
	// version still can be decoded.
	secret := &Secret{Current: latest.Payload.Data}
	if previous != nil {
		secret.Previous = [][]byte{previous.Payload.Data}
	}
	return secret, nil
}

// Close is part of Source interface.
func (s *SecretManagerSource) Close() error {
	return s.client.Close()
}
