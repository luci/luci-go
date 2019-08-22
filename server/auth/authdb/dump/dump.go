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
	"io/ioutil"
	"net/http"
	"time"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"

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
// to make RPCs to Auth Service to discover for them).
//
// The only time when Auth Service is hit directly is when reading from the GCS
// results in permission errors. When this happens, Fetcher tries to authorize
// itself through Auth Service API call and then retries the fetch.
type Fetcher struct {
	StorageDumpPath    string   // GCS storage path to the dump: "<bucket>/<object>"
	AuthServiceURL     string   // URL of the auth service: "https://..."
	AuthServiceAccount string   // service account name that signed the blob
	OAuthScopes        []string // scopes to use when making OAuth tokens

	storageURL string       // Google Storage URL, mocked in tests
	client     *http.Client // lazy-initialized authenticating http.Client, also mocked in tests
}

// FetchAuthDB checks whether there's a newer version of AuthDB available in
// GCS and fetches it if so. If 'cur' is already up-to-date, returns it as is.
//
// Logs and retries errors internally until the context cancellation or timeout.
func (f *Fetcher) FetchAuthDB(c context.Context, cur *authdb.SnapshotDB) (fresh *authdb.SnapshotDB, err error) {
	err = retry.Retry(c, transient.Only(indefiniteRetry), func() (err error) {
		fresh, err = f.doFetchAttempt(c, cur)
		return err
	}, func(err error, wait time.Duration) {
		logging.Warningf(c, "Failed to fetch AuthDB dump, will retry in %s: %s", wait, err)
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
func (f *Fetcher) doFetchAttempt(c context.Context, cur *authdb.SnapshotDB) (*authdb.SnapshotDB, error) {
	latestRev, needAccess, err := f.fetchLatestRev(c)
	if err != nil {
		return nil, err
	}

	// If have no access, ask for it and immediately try again.
	if needAccess {
		if err := f.requestAccess(c); err != nil {
			return nil, err
		}
		switch latestRev, needAccess, err = f.fetchLatestRev(c); {
		case err != nil:
			return nil, err
		case needAccess:
			return nil, errors.Reason("still no access to GCS").Tag(transient.Tag).Err()
		}
	}

	// Skip the rest if we already have the latest revision.
	if cur != nil && cur.Rev >= latestRev {
		return cur, nil
	}

	// Fetch and validate the new snapshot.
	logging.Infof(c, "AuthDB rev %d is available, fetching it...", latestRev)
	signed, err := f.fetchSignedAuthDB(c)
	if err != nil {
		return nil, err
	}
	if err := f.checkSignature(c, signed); err != nil {
		return nil, err
	}
	fresh, err := f.deserializeAuthDB(c, signed.AuthDbBlob)
	if err != nil {
		return nil, err
	}

	// Make sure we don't go to an older revisions no matter what.
	if cur != nil && fresh.Rev <= cur.Rev {
		logging.Errorf(c, "Unexpectedly got an older snapshot (%d <= %d), ignoring it", fresh.Rev, cur.Rev)
		return cur, nil
	}
	return fresh, nil
}

// fetchLatestRev returns the revision of the latest AuthDB dump in the storage.
//
// On access errors returns (0, true, nil). All other errors are considered
// transient.
func (f *Fetcher) fetchLatestRev(c context.Context) (rev int64, needAccess bool, err error) {
	switch code, resp, err := f.fetchFromGCS(c, "latest.json"); {
	case err != nil:
		return 0, false, transient.Tag.Apply(err)
	case code == http.StatusOK:
		rev := protocol.AuthDBRevision{}
		if err := jsonpb.Unmarshal(bytes.NewReader(resp), &rev); err != nil {
			return 0, false, errors.Annotate(err, "failed to unmarshal AuthDBRevision").Err()
		}
		return rev.AuthDbRev, false, nil
	case code == http.StatusForbidden || code == http.StatusNotFound:
		logging.Errorf(c, "Permission denied when fetching latest.json")
		return 0, true, nil
	default:
		return 0, false, errors.Reason("got HTTP %d when fetching latest.json:\n%s", code, resp).Tag(transient.Tag).Err()
	}
}

// fetchSignedAuthDB fetches SignedAuthDB from GCS and deserializes it.
func (f *Fetcher) fetchSignedAuthDB(c context.Context) (*protocol.SignedAuthDB, error) {
	code, resp, err := f.fetchFromGCS(c, "latest.db")
	switch {
	case err != nil:
		return nil, transient.Tag.Apply(err)
	case code == http.StatusOK:
		logging.Infof(c, "Fetched AuthDB snapshot (%.1f Kb)", float32(len(resp))/1024)
		db := protocol.SignedAuthDB{}
		if err := proto.Unmarshal(resp, &db); err != nil {
			return nil, errors.Annotate(err, "failed to unmarshal SignedAuthDB").Err()
		}
		return &db, nil
	default:
		return nil, errors.Reason("got HTTP %d when fetching latest.json:\n%s", code, resp).Tag(transient.Tag).Err()
	}
}

// checkSignature checks the signature in SignedAuthDB.
func (f *Fetcher) checkSignature(c context.Context, s *protocol.SignedAuthDB) error {
	if s.SignerId != f.AuthServiceAccount {
		return errors.Reason("the snapshot is signed by %q, but we accept only %q", s.SignerId, f.AuthServiceAccount).Err()
	}
	certs, err := signing.FetchCertificatesForServiceAccount(c, s.SignerId)
	if err != nil {
		return errors.Annotate(err, "failed to fetch certs of %q", s.SignerId).Tag(transient.Tag).Err()
	}
	hash := sha512.Sum512(s.AuthDbBlob)
	return certs.CheckSignature(s.SigningKeyId, hash[:], s.Signature)
}

// deserializeAuthDB unmarshals and validates AuthDB stored in serialized
// ReplicationPushRequest.
func (f *Fetcher) deserializeAuthDB(c context.Context, blob []byte) (*authdb.SnapshotDB, error) {
	m := protocol.ReplicationPushRequest{}
	if err := proto.Unmarshal(blob, &m); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal ReplicationPushRequest").Err()
	}
	rev := m.Revision.AuthDbRev
	logging.Infof(c,
		"AuthDB snapshot rev %d generated by %s (using components.auth v%s)",
		rev, m.Revision.PrimaryId, m.AuthCodeVersion)
	snap, err := authdb.NewSnapshotDB(m.AuthDb, f.AuthServiceURL, rev, true)
	if err != nil {
		return nil, errors.Annotate(err, "snapshot at rev %d fails validation", rev).Err()
	}
	return snap, nil
}

// requestAccess asks Auth Service to grant us access to the AuthDB dump.
func (f *Fetcher) requestAccess(c context.Context) error {
	svc := service.AuthService{
		URL:         f.AuthServiceURL,
		OAuthScopes: f.OAuthScopes, // use same scopes as for GCS to reuse the cached token
	}
	logging.Warningf(c, "Asking %s to grant us access to read %q...", f.AuthServiceURL, f.StorageDumpPath)
	switch info, err := svc.RequestAccess(c); {
	case err != nil:
		return transient.Tag.Apply(err)
	case info.StorageDumpPath == "":
		return errors.Reason("service %s is not configured to upload AuthDB to GCS", f.AuthServiceURL).Err()
	case info.StorageDumpPath != f.StorageDumpPath:
		// Note: we can't just dynamically "fix" f.StorageDumpPath. It is important
		// that original configuration (e.g. CLI flag) is fixed too, otherwise after
		// restart we'll resume looking at the wrong place. So treat this situation
		// as a fatal error.
		return errors.Reason("wrong configuration: service %s uploads AuthDB to %q, but we are looking at %q",
			f.AuthServiceURL, info.StorageDumpPath, f.StorageDumpPath).Err()
	default:
		logging.Infof(c, "Access granted")
		return nil
	}
}

// fetchFromGCS fetches gs://<StorageDumpPath>/<rel> file into memory.
func (f *Fetcher) fetchFromGCS(c context.Context, rel string) (statusCode int, body []byte, err error) {
	cl, err := f.authClient(c)
	if err != nil {
		return 0, nil, err
	}

	storageURL := "https://storage.googleapis.com"
	if f.storageURL != "" {
		storageURL = f.storageURL
	}
	url := fmt.Sprintf("%s/%s/%s", storageURL, f.StorageDumpPath, rel)

	req, _ := http.NewRequest("GET", url, nil)
	resp, err := cl.Do(req.WithContext(c))
	if err != nil {
		return 0, nil, errors.Annotate(err, "GET %s", url).Err()
	}
	defer resp.Body.Close()

	blob, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, errors.Annotate(err, "GET %s", url).Err()
	}
	return resp.StatusCode, blob, nil
}

// authClient returns authenticating http.Client to use for GCS access.
func (f *Fetcher) authClient(c context.Context) (*http.Client, error) {
	if f.client == nil {
		t, err := auth.GetRPCTransport(c, auth.AsSelf, auth.WithScopes(f.OAuthScopes...))
		if err != nil {
			return nil, err
		}
		f.client = &http.Client{Transport: t}
	}
	return f.client, nil
}
