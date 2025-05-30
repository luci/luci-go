// Copyright 2019 The LUCI Authors.
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

// Package dump implements loading AuthDB from dumps in Google Storage.
package dump

import (
	"bytes"
	"context"
	"crypto/sha512"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/service"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/auth/signing"
)

// Fetcher can fetch AuthDB snapshots from GCS dumps, requesting access through
// Auth Service if necessary.
//
// It's designed not to depend on Auth Service availability at all if everything
// is already setup (i.e. the access to AuthDB snapshot is granted). For that
// reason it requires the location of GCS dump and name of Auth Service's
// signing account to be provided as static configuration (since we don't want
// to make RPCs to potentially unavailable Auth Service to discover them).
//
// The only time Auth Service is directly hit is when GCS returns permission
// errors. When this happens, Fetcher tries to authorize itself through the
// Auth Service API call and then retries the fetch.
type Fetcher struct {
	StorageDumpPath    string   // GCS storage path to the dump "<bucket>/<object>"
	AuthServiceURL     string   // URL of the auth service "https://..."
	AuthServiceAccount string   // service account name that signed the blob
	OAuthScopes        []string // scopes to use when making OAuth tokens

	testRetryPolicy   func() retry.Iterator       // how to retry, mocked in tests
	testStorageURL    string                      // Google Storage URL, mocked in tests
	testStorageClient *http.Client                // client to access Google Storage, mocked in tests
	testSigningCerts  *signing.PublicCertificates // certs to use to check signature, mocked in tests
}

// FetchAuthDB checks whether there's a newer version of AuthDB available in
// GCS and fetches it if so. If 'cur' is already up-to-date, returns it as is.
//
// Logs and retries errors internally until the context cancellation or timeout.
func (f *Fetcher) FetchAuthDB(ctx context.Context, cur *authdb.SnapshotDB) (fresh *authdb.SnapshotDB, err error) {
	client := f.testStorageClient
	if client == nil {
		t, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(f.OAuthScopes...))
		if err != nil {
			return nil, errors.New("can't get authenticating transport")
		}
		client = &http.Client{Transport: t}
	}

	retryPolicy := f.testRetryPolicy
	if retryPolicy == nil {
		retryPolicy = transient.Only(indefiniteRetry)
	}

	err = retry.Retry(ctx, retryPolicy, func() (err error) {
		fresh, err = f.doFetchAttempt(ctx, cur, client)
		return err
	}, func(err error, wait time.Duration) {
		logging.Warningf(ctx, "Failed to fetch AuthDB dump, will retry in %s: %s", wait, err)
	})
	return
}

// indefiniteRetry is retry.Iterator that retries indefinitely.
func indefiniteRetry() retry.Iterator {
	return &retry.ExponentialBackoff{
		Limited: retry.Limited{
			Retries: -1,
			Delay:   500 * time.Millisecond,
		},
		MaxDelay: 30 * time.Second,
	}
}

// doFetchAttempt is one iteration of FetchAuthDB retry loop.
func (f *Fetcher) doFetchAttempt(ctx context.Context, cur *authdb.SnapshotDB, client *http.Client) (*authdb.SnapshotDB, error) {
	// Fetch a tiny latest.json. In most cases this is the only RPC we'll do.
	latestRev, needAccess, err := f.fetchLatestRev(ctx, client)
	if err != nil {
		return nil, err
	}

	// If have no access, ask for it and immediately try again.
	if needAccess {
		if err := f.requestAccess(ctx); err != nil {
			return nil, err
		}
		switch latestRev, needAccess, err = f.fetchLatestRev(ctx, client); {
		case err != nil:
			return nil, err
		case needAccess: // this should not be happening
			return nil, transient.Tag.Apply(errors.New("still no access to GCS"))
		}
	}

	// Skip the rest if we already have same or more recent revision.
	if cur != nil && cur.Rev >= latestRev {
		if cur.Rev > latestRev {
			logging.Warningf(ctx, "AuthDB dump revision went back in time (we have %d, the dump is %d)", cur.Rev, latestRev)
		}
		return cur, nil
	}

	// Fetch and validate the new snapshot.
	logging.Infof(ctx, "AuthDB rev %d is available, fetching it...", latestRev)
	signed, err := f.fetchSignedAuthDB(ctx, client)
	if err != nil {
		return nil, err
	}
	if err := f.checkSignature(ctx, signed); err != nil {
		return nil, err
	}
	fresh, err := f.deserializeAuthDB(ctx, signed.AuthDbBlob)
	if err != nil {
		return nil, err
	}

	// Make sure we don't switch to an older revisions no matter what. This should
	// not be happening.
	if cur != nil && fresh.Rev <= cur.Rev {
		logging.Errorf(ctx, "Unexpectedly got an older snapshot (%d <= %d), ignoring it", fresh.Rev, cur.Rev)
		return cur, nil
	}
	return fresh, nil
}

// fetchLatestRev returns the revision of the latest AuthDB dump in the storage.
//
// On access errors returns (0, true, nil). All other errors are considered
// transient.
func (f *Fetcher) fetchLatestRev(ctx context.Context, client *http.Client) (rev int64, needAccess bool, err error) {
	switch code, resp, err := f.fetchFromGCS(ctx, client, "latest.json"); {
	case err != nil:
		return 0, false, transient.Tag.Apply(err)
	case code == http.StatusOK:
		rev := protocol.AuthDBRevision{}
		if err := jsonpb.Unmarshal(bytes.NewReader(resp), &rev); err != nil {
			return 0, false, errors.Fmt("failed to unmarshal AuthDBRevision: %w", err)
		}
		return rev.AuthDbRev, false, nil
	case code == http.StatusForbidden || code == http.StatusNotFound:
		logging.Errorf(ctx, "Permission errors when fetching latest.json")
		return 0, true, nil
	default:
		return 0, false, transient.Tag.Apply(errors.Fmt("got HTTP %d when fetching latest.json:\n%s", code, resp))
	}
}

// fetchSignedAuthDB fetches SignedAuthDB from GCS and deserializes it.
func (f *Fetcher) fetchSignedAuthDB(ctx context.Context, client *http.Client) (*protocol.SignedAuthDB, error) {
	code, resp, err := f.fetchFromGCS(ctx, client, "latest.db")
	switch {
	case err != nil:
		return nil, transient.Tag.Apply(err)
	case code == http.StatusOK:
		logging.Infof(ctx, "Fetched AuthDB snapshot (%.1f Kb)", float32(len(resp))/1024)
		db := protocol.SignedAuthDB{}
		if err := proto.Unmarshal(resp, &db); err != nil {
			return nil, errors.Fmt("failed to unmarshal SignedAuthDB: %w", err)
		}
		return &db, nil
	default:
		return nil, transient.Tag.Apply(errors.Fmt("got HTTP %d when fetching latest.json:\n%s", code, resp))
	}
}

// checkSignature checks the signature in SignedAuthDB.
func (f *Fetcher) checkSignature(ctx context.Context, s *protocol.SignedAuthDB) error {
	if s.SignerId != f.AuthServiceAccount {
		return errors.Fmt("the snapshot is signed by %q, but we accept only %q", s.SignerId, f.AuthServiceAccount)
	}

	certs := f.testSigningCerts
	if certs == nil {
		var err error
		if certs, err = signing.FetchCertificatesForServiceAccount(ctx, s.SignerId); err != nil {
			return transient.Tag.Apply(errors.Fmt("failed to fetch certs of %q: %w", s.SignerId, err))
		}
	}

	hash := sha512.Sum512(s.AuthDbBlob)
	if err := certs.CheckSignature(s.SigningKeyId, hash[:], s.Signature); err != nil {
		return errors.Fmt("failed to verify that AuthDB was signed by %q: %w", s.SignerId, err)
	}
	return nil
}

// deserializeAuthDB unmarshals and validates AuthDB stored in serialized
// ReplicationPushRequest.
func (f *Fetcher) deserializeAuthDB(ctx context.Context, blob []byte) (*authdb.SnapshotDB, error) {
	m := protocol.ReplicationPushRequest{}
	if err := proto.Unmarshal(blob, &m); err != nil {
		return nil, errors.Fmt("failed to unmarshal ReplicationPushRequest: %w", err)
	}
	rev := m.Revision.AuthDbRev
	logging.Infof(ctx,
		"AuthDB snapshot rev %d generated by %s (using components.auth v%s)",
		rev, m.Revision.PrimaryId, m.AuthCodeVersion)
	snap, err := authdb.NewSnapshotDB(m.AuthDb, f.AuthServiceURL, rev, true)
	if err != nil {
		return nil, errors.Fmt("snapshot at rev %d fails validation: %w", rev, err)
	}
	return snap, nil
}

// requestAccess asks Auth Service to grant us access to the AuthDB dump.
func (f *Fetcher) requestAccess(ctx context.Context) error {
	svc := service.AuthService{
		URL:         f.AuthServiceURL,
		OAuthScopes: f.OAuthScopes, // use same scopes as for GCS to reuse the cached token
	}
	logging.Warningf(ctx, "Asking %s to grant us access to read %q...", f.AuthServiceURL, f.StorageDumpPath)
	switch info, err := svc.RequestAccess(ctx); {
	case err != nil:
		return transient.Tag.Apply(err)
	case info.StorageDumpPath == "":
		return errors.Fmt("service %s is not configured to upload AuthDB to GCS", f.AuthServiceURL)
	case info.StorageDumpPath != f.StorageDumpPath:
		// Note: we can't just dynamically "fix" f.StorageDumpPath. It is important
		// that original configuration (e.g. CLI flag) is fixed too, otherwise after
		// restart we'll resume looking at the wrong place. So treat this situation
		// as a fatal error.
		return errors.Fmt("wrong configuration: service %s uploads AuthDB to %q, but we are looking at %q",
			f.AuthServiceURL, info.StorageDumpPath, f.StorageDumpPath)
	default:
		logging.Infof(ctx, "Access granted")
		return nil
	}
}

// fetchFromGCS fetches gs://<StorageDumpPath>/<rel> file into memory.
func (f *Fetcher) fetchFromGCS(ctx context.Context, client *http.Client, rel string) (statusCode int, body []byte, err error) {
	storageURL := "https://storage.googleapis.com"
	if f.testStorageURL != "" {
		storageURL = f.testStorageURL
	}
	url := fmt.Sprintf("%s/%s/%s", storageURL, f.StorageDumpPath, rel)

	req, _ := http.NewRequest("GET", url, nil)
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return 0, nil, errors.Fmt("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	blob, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, errors.Fmt("GET %s: %w", url, err)
	}
	return resp.StatusCode, blob, nil
}
