// Copyright 2021 The LUCI Authors.
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

package gaemiddleware

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/insecurecleartextkeyset"
	"github.com/google/tink/go/keyset"
	"github.com/google/tink/go/tink"
	"google.golang.org/api/option"
	"google.golang.org/appengine"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/caching"
)

var (
	cachedAEAD          = caching.RegisterCacheSlot()
	settingsCheckPeriod = time.Minute
	rotationCheckPeriod = time.Hour
)

// AEADProvider loads the primary encryption key from Google Secret Manager.
//
// If it is not configured and we are running on a dev server, generates a new
// phony one. If it is not configured and we are running in production, returns
// nil to indicate AEAD is not available.
//
// If the key is configured, but can't be loaded, returns a tink.AEAD
// implementation that returns errors in all its methods.
func AEADProvider(ctx context.Context) tink.AEAD {
	s := fetchCachedSettings(ctx)
	if s.EncryptionKey == "" {
		if appengine.IsDevAppServer() {
			return useDevServerKey(ctx)
		}
		return nil
	}
	state, err := cachedAEAD.Fetch(ctx, func(prev any) (updated any, exp time.Duration, err error) {
		state, _ := prev.(*aeadCachedState)
		if state == nil || state.keyPath != s.EncryptionKey {
			state = &aeadCachedState{keyPath: s.EncryptionKey}
		}
		err = state.refresh(ctx)
		return state, settingsCheckPeriod, err
	})
	if err != nil {
		return brokenAEAD{err}
	}
	return state.(*aeadCachedState).aead
}

type brokenAEAD struct {
	err error
}

func (b brokenAEAD) Encrypt(_, _ []byte) ([]byte, error) { return nil, b.err }
func (b brokenAEAD) Decrypt(_, _ []byte) ([]byte, error) { return nil, b.err }

////////////////////////////////////////////////////////////////////////////////
// Prod caching helpers.

type aeadCachedState struct {
	keyPath       string
	rotationCheck time.Time
	aead          tink.AEAD
}

func (s *aeadCachedState) refresh(ctx context.Context) error {
	if s.aead != nil && clock.Now(ctx).Before(s.rotationCheck) {
		return nil // have the key and it is fresh enough
	}

	// Fetch the secret blob from the Secret Manager.
	chunks := strings.Split(strings.TrimPrefix(s.keyPath, "sm://"), "/")
	if len(chunks) != 2 {
		logging.Errorf(ctx, "Bad encryption key URI %q", s.keyPath)
		return errors.Fmt("bad secret URI %q", s.keyPath)
	}
	blob, err := fetchSecret(ctx, chunks[0], chunks[1])
	if err != nil {
		logging.Errorf(ctx, "Failed to load Google Secret Manager secret %s: %s", s.keyPath, err)
		return err
	}

	// Construct tink.AEAD out of it.
	kh, err := insecurecleartextkeyset.Read(keyset.NewJSONReader(bytes.NewReader(blob)))
	if err != nil {
		logging.Errorf(ctx, "Secret %q doesn't contain a valid Tink keyset: %s", s.keyPath, err)
		return err
	}
	a, err := aead.New(kh)
	if err != nil {
		logging.Errorf(ctx, "Secret %q doesn't contain an AEAD Tink key: %s", s.keyPath, err)
		return err
	}

	// Record when we should check it again in case it is rotated.
	s.aead = a
	s.rotationCheck = clock.Now(ctx).Add(rotationCheckPeriod)
	return nil
}

func fetchSecret(ctx context.Context, project, secret string) ([]byte, error) {
	ts, err := auth.GetTokenSource(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Fmt("failed to get OAuth2 token source: %w", err)
	}

	client, err := secretmanager.NewClient(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, errors.Fmt("failed to setup Secret Manager client: %w", err)
	}
	defer client.Close()

	latest, err := client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", project, secret),
	})
	if err != nil {
		return nil, err
	}
	logging.Infof(ctx, "Loaded secret %q", latest.Name)
	return latest.Payload.Data, nil
}

////////////////////////////////////////////////////////////////////////////////
// Dev server helpers.

var devServerLock sync.Mutex

func useDevServerKey(ctx context.Context) tink.AEAD {
	devServerLock.Lock()
	defer devServerLock.Unlock()

	path := filepath.Join(os.TempDir(), "luci-insecure-dev-tink-aead-key.json")

	// Try to load an existing key.
	switch key, err := loadDevServerKey(path); {
	case err == nil:
		return key
	case !os.IsNotExist(err):
		logging.Warningf(ctx, "Ignoring bad dev server Tink key %s: %s", path, err)
	}

	// Generate the new key.
	kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		panic(err) // e.g. no entropy
	}
	out, err := aead.New(kh)
	if err != nil {
		panic(err) // not really possible
	}
	buf := &bytes.Buffer{}
	if err = insecurecleartextkeyset.Write(kh, keyset.NewJSONWriter(buf)); err != nil {
		panic(err) // not really possible
	}

	// Store it so encrypted blobs survive the dev server restart.
	logging.Infof(ctx, "Generated new dev server Tink key at %s", path)
	if err := os.WriteFile(path, buf.Bytes(), 0600); err != nil {
		logging.Warningf(ctx, "Failed to store dev server Tink key %s: %s", path, err)
	}

	return out
}

func loadDevServerKey(path string) (tink.AEAD, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	kh, err := insecurecleartextkeyset.Read(keyset.NewJSONReader(bytes.NewReader(blob)))
	if err != nil {
		return nil, err
	}
	return aead.New(kh)
}
