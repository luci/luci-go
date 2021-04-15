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
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	gax "github.com/googleapis/gax-go/v2"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// SecretManagerStore implements Store using Google Secret Manager.
//
// Stored secrets are fetched directly from Google Secret Manager. Random
// secrets are derived from a root secret using HKDF via DerivedStore.
type SecretManagerStore struct {
	// CloudProject is used for loading secrets of the form "sm://<name>".
	CloudProject string
	// AccessSecretVersion is an RPC to fetch the secret from the Secret Manager.
	AccessSecretVersion func(context.Context, *secretmanagerpb.AccessSecretVersionRequest, ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error)

	rootSecret    *trackedSecret // the secret loaded in LoadRootSecret
	randomSecrets *DerivedStore  // the store used by RandomSecret

	handlersM sync.RWMutex
	handlers  map[string][]RotationHandler
}

// LoadRootSecret loads the root secret used to generate random secrets.
//
// See StoredSecret for the format of the root secret. Must be called before
// MaintenanceLoop.
func (sm *SecretManagerStore) LoadRootSecret(ctx context.Context, rootSecret string) error {
	rootSecret, err := sm.normalizeName(rootSecret)
	if err != nil {
		return errors.Annotate(err, "bad root secret name").Err()
	}
	secret, err := sm.readSecret(ctx, rootSecret, true)
	if err != nil {
		return errors.Annotate(err, "failed to read the initial value of the root secret").Err()
	}
	secret.logActiveVersions(ctx)
	secret.logNextReloadTime(ctx)
	sm.rootSecret = secret
	sm.randomSecrets = NewDerivedStore(secret.value)
	sm.AddRotationHandler(ctx, secret.name, func(_ context.Context, secret Secret) {
		sm.randomSecrets.SetRoot(secret)
	})
	return nil
}

// MaintenanceLoop runs a loop that periodically rereads secrets.
//
// It exits on context cancellation. Logs errors inside.
func (sm *SecretManagerStore) MaintenanceLoop(ctx context.Context) {
	rootSecret := sm.rootSecret
	if rootSecret == nil || rootSecret.nextReload.IsZero() {
		return // not using derived secrets at all or using a static root secret
	}
	for {
		sleep := rootSecret.nextReload.Sub(clock.Now(ctx))
		if r := <-clock.After(ctx, sleep); r.Err != nil {
			return // the context is canceled
		}
		if sm.tryReloadSecret(ctx, rootSecret) {
			sm.notifyUpdateHandlers(ctx, rootSecret.name, rootSecret.value)
		}
	}
}

// RandomSecret returns a random secret given its name.
func (sm *SecretManagerStore) RandomSecret(ctx context.Context, name string) (Secret, error) {
	if sm.randomSecrets == nil {
		return Secret{}, errors.New("the root secret of the secret store is not initialized")
	}
	return sm.randomSecrets.RandomSecret(ctx, name)
}

// StoredSecret returns a previously stored secret given its name.
//
// Value of `name` should have form:
//   * `sm://<project>/<secret>`: a concrete secret in Google Secret Manager.
//   * `sm://<secret>`: same as `sm://<CloudProject>/<secret>`.
//   * `devsecret://<base64-encoded secret>`: return this concrete secret.
//   * `devsecret-text://<string>`: return this concrete secret.
func (sm *SecretManagerStore) StoredSecret(ctx context.Context, name string) (Secret, error) {
	name, err := sm.normalizeName(name)
	if err != nil {
		return Secret{}, err
	}
	return Secret{}, errors.New("not implemented yet")
}

// AddRotationHandler registers a callback which is called when the stored
// secret is updated.
//
// The handler is called from an internal goroutine and receives a context
// passed to MaintenanceLoop.
func (sm *SecretManagerStore) AddRotationHandler(ctx context.Context, name string, cb RotationHandler) error {
	switch name, err := sm.normalizeName(name); {
	case err != nil:
		return err

	case !strings.HasPrefix(name, "sm://"):
		return nil // no updates for static secrets

	default:
		sm.handlersM.Lock()
		defer sm.handlersM.Unlock()
		if sm.handlers == nil {
			sm.handlers = make(map[string][]RotationHandler, 1)
		}
		sm.handlers[name] = append(sm.handlers[name], cb)
		return nil
	}
}

////////////////////////////////////////////////////////////////////////////////

const (
	// Randomized secret re-reading interval. It is pretty big, since secrets are
	// assumed to be rotated infrequently. In rare emergencies a service can be
	// restarted to pick new secrets faster.
	reloadIntervalMin = 2 * time.Hour
	reloadIntervalMax = 4 * time.Hour

	// Max delay when retrying failing fetches.
	maxRetryDelay = 30 * time.Minute
)

// trackedSecret is a secret that is periodically reread in MaintenanceLoop.
type trackedSecret struct {
	name            string    // the name it was loaded under
	withPrevVersion bool      // true to fetch the previous version
	value           Secret    // the actual value
	versions        [2]int64  // the latest and one previous (if enabled) version numbers
	attempts        int       // how many consecutive times we failed to reload the secret
	nextReload      time.Time // when we should reload the secret or zero if reloads are disabled
}

func (s *trackedSecret) logActiveVersions(ctx context.Context) {
	if s.versions != [2]int64{0, 0} {
		formatVer := func(v int64) string {
			if v == 0 {
				return "none"
			}
			return strconv.FormatInt(v, 10)
		}
		logging.Infof(ctx, "Loaded secret %q, active versions latest=%s, previous=%s",
			s.name,
			formatVer(s.versions[0]),
			formatVer(s.versions[1]),
		)
	}
}

func (s *trackedSecret) logNextReloadTime(ctx context.Context) {
	if !s.nextReload.IsZero() {
		logging.Debugf(ctx, "Will attempt to reload the secret %q in %s", s.name, s.nextReload.Sub(clock.Now(ctx)))
	}
}

// normalizeName check the secret name format and normalizes it.
func (sm *SecretManagerStore) normalizeName(name string) (string, error) {
	switch {
	case strings.HasPrefix(name, "devsecret://"):
		return name, nil

	case strings.HasPrefix(name, "devsecret-text://"):
		return name, nil

	case strings.HasPrefix(name, "sm://"):
		switch parts := strings.Split(strings.TrimPrefix(name, "sm://"), "/"); {
		case len(parts) == 1:
			if sm.CloudProject == "" {
				return "", errors.Reason("can't use secret reference %q when the Cloud Project name is not configured", name).Err()
			}
			return fmt.Sprintf("sm://%s/%s", sm.CloudProject, parts[0]), nil
		case len(parts) == 2:
			return name, nil
		default:
			return "", errors.Reason("sm:// secret reference should have form sm://<name> or sm://<project>/<name>").Err()
		}

	default:
		return "", errors.Reason("not supported secret reference %q", name).Err()
	}
}

// readSecret fetches a secret given its normalized name.
func (sm *SecretManagerStore) readSecret(ctx context.Context, name string, withPrevVersion bool) (*trackedSecret, error) {
	switch {
	case strings.HasPrefix(name, "devsecret://"):
		value, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(name, "devsecret://"))
		if err != nil {
			return nil, errors.Annotate(err, "bad devsecret://, not base64 encoding").Err()
		}
		return &trackedSecret{
			name:  name,
			value: Secret{Current: value},
		}, nil

	case strings.HasPrefix(name, "devsecret-text://"):
		return &trackedSecret{
			name:  name,
			value: Secret{Current: []byte(strings.TrimPrefix(name, "devsecret-text://"))},
		}, nil

	case strings.HasPrefix(name, "sm://"):
		return sm.readSecretFromGSM(ctx, name, withPrevVersion)

	default:
		panic("impossible, already checked in normalizeSecretName")
	}
}

// readSecretFromGSM returns a sm://... secret given its normalized name.
func (sm *SecretManagerStore) readSecretFromGSM(ctx context.Context, name string, withPrevVersion bool) (*trackedSecret, error) {
	logging.Debugf(ctx, "Loading secret %q", name)

	// `name` here must have sm://<project>/<secret> format.
	parts := strings.Split(strings.TrimPrefix(name, "sm://"), "/")
	if len(parts) != 2 {
		panic("impossible, should be normalize already")
	}
	project, secret := parts[0], parts[1]

	// The latest version is used as the primary active version.
	latest, err := sm.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", project, secret),
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to get the latest version of the secret %q", name).Err()
	}

	// The version name has format "projects/.../secrets/.../versions/<number>".
	// We want to grab the version number to peek at the previous one (if any).
	// Note that GSM uses numeric project IDs in `latest.Name` instead of Cloud
	// Project names, so we can't just trim the prefix.
	idx := strings.LastIndex(latest.Name, "/")
	if idx == -1 {
		return nil, errors.Reason("unexpected version name format %q", latest.Name).Err()
	}
	version, err := strconv.ParseInt(latest.Name[idx+1:], 10, 64)
	if err != nil {
		return nil, errors.Reason("unexpected version name format %q", latest.Name).Err()
	}

	result := &trackedSecret{
		name:            name,
		withPrevVersion: withPrevVersion,
		value:           Secret{Current: latest.Payload.Data},
		versions:        [2]int64{version, 0}, // note: GSM versions start with 1
		nextReload:      nextReloadTime(ctx),
	}
	if !withPrevVersion || version == 1 {
		return result, nil
	}

	// Potentially have a previous version. Try to grab it.
	prevVersion := version - 1
	previous, err := sm.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/%d", project, secret, prevVersion),
	})
	switch status.Code(err) {
	case codes.OK:
		result.value.Previous = [][]byte{previous.Payload.Data}
		result.versions[1] = prevVersion
		return result, nil
	case codes.FailedPrecondition, codes.NotFound:
		return result, nil // already disabled or deleted, this is fine
	default:
		return nil, errors.Annotate(err, "failed to read the previous version %d of the secret %q", prevVersion, name).Err()
	}
}

// tryReloadSecret attempts to reload the secret, mutating it in-place.
//
// Returns true if the secret has a new value now or false if the value didn't
// change (either we failed to fetch it or it really didn't change).
//
// Logs errors inside. Fields `secret.attempts` and `secret.nextReload` are
// mutated even on failures.
func (sm *SecretManagerStore) tryReloadSecret(ctx context.Context, secret *trackedSecret) bool {
	fresh, err := sm.readSecret(ctx, secret.name, secret.withPrevVersion)
	if err != nil {
		secret.attempts += 1
		sleep := reloadBackoffInterval(ctx, secret.attempts)
		logging.Errorf(ctx, "Failed to reload the secret (attempt %d, next try in %s): %s", secret.attempts, sleep, err)
		secret.nextReload = clock.Now(ctx).Add(sleep)
		return false
	}
	updated := !fresh.value.Equal(secret.value)
	*secret = *fresh
	if updated {
		secret.logActiveVersions(ctx)
	}
	secret.logNextReloadTime(ctx)
	return updated
}

// notifyUpdateHandlers calls callback registered by RegisterUpdateHandler.
func (sm *SecretManagerStore) notifyUpdateHandlers(ctx context.Context, name string, val Secret) {
	sm.handlersM.RLock()
	cbs := append([]RotationHandler(nil), sm.handlers[name]...)
	sm.handlersM.RUnlock()
	for _, cb := range cbs {
		cb(ctx, val)
	}
}

// nextReloadTime returns a time when we should try to reload the secret.
func nextReloadTime(ctx context.Context) time.Time {
	dt := reloadIntervalMin + time.Duration(mathrand.Int63n(ctx, int64(reloadIntervalMax-reloadIntervalMin)))
	return clock.Now(ctx).Add(dt)
}

// reloadBackoffInterval tells how long to sleep after a failed reload attempt.
func reloadBackoffInterval(ctx context.Context, attempt int) time.Duration {
	factor := math.Pow(2.0, float64(attempt)) // 2, 4, 8, ...
	factor += 10 * mathrand.Float64(ctx)
	dur := time.Duration(float64(time.Second) * factor)
	if dur > maxRetryDelay {
		dur = maxRetryDelay
	}
	return dur
}
