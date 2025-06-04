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
	"bytes"
	"container/heap"
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/google/tink/go/aead"
	"github.com/google/tink/go/insecurecleartextkeyset"
	"github.com/google/tink/go/keyset"
	gax "github.com/googleapis/gax-go/v2"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

// The version of the loaded secret per alias.
var versionMetric = metric.NewInt(
	"secrets/gsm/version",
	"Version number of a currently loaded Google Secret Manager secret",
	&types.MetricMetadata{},
	field.String("project"), // GCP project with the secret
	field.String("secret"),  // the name of the secret
	field.String("alias"),   // one of "current", "previous", "next"
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

	randomSecrets Store // the store used by RandomSecret

	rwm           sync.RWMutex
	secretsByName map[string]*trackedSecret
	secretsByTime trackedSecretsPQ
	wakeUp        chan struct{}
	handlers      map[string][]RotationHandler

	testingEvents chan string // used in tests
}

// LoadRootSecret loads the root secret used to generate random secrets.
//
// See StoredSecret for the format of the root secret.
func (sm *SecretManagerStore) LoadRootSecret(ctx context.Context, rootSecret string) error {
	secret, err := sm.StoredSecret(ctx, rootSecret)
	if err != nil {
		return errors.Fmt("failed to read the initial value of the root secret: %w", err)
	}
	derivedStore := NewDerivedStore(secret)
	sm.AddRotationHandler(ctx, rootSecret, func(_ context.Context, secret Secret) {
		derivedStore.SetRoot(secret)
	})
	sm.SetRandomSecretsStore(derivedStore)
	return nil
}

// SetRandomSecretsStore changes the store used for RandomSecret(...).
//
// Can be used instead of LoadRootSecret to hook up a custom implementation.
func (sm *SecretManagerStore) SetRandomSecretsStore(s Store) {
	sm.randomSecrets = s
}

// MaintenanceLoop runs a loop that periodically rereads secrets.
//
// It exits on context cancellation. Logs errors inside.
func (sm *SecretManagerStore) MaintenanceLoop(ctx context.Context) {
	wg := sync.WaitGroup{}
	defer wg.Wait()

	for ctx.Err() == nil {
		var nextReload time.Time
		var wakeUp chan struct{}

		sm.emitTestingEvent(ctx, "checking")

		sm.rwm.Lock()
		for sm.reloadNextSecretLocked(ctx, &wg) {
		}
		if len(sm.secretsByTime) != 0 {
			nextReload = sm.secretsByTime[0].nextReload
		}
		wakeUp = make(chan struct{})
		sm.wakeUp = wakeUp // closed in StoredSecret
		sm.rwm.Unlock()

		sm.emitTestingEvent(ctx, "sleeping")
		if !nextReload.IsZero() {
			sleep := nextReload.Sub(clock.Now(ctx))
			logging.Debugf(ctx, "Sleeping %s until the next scheduled refresh", sleep)
			select {
			case <-wakeUp:
				sm.emitTestingEvent(ctx, "woken")
			case <-clock.After(ctx, sleep):
				sm.emitTestingEvent(ctx, "slept %s", sleep.Round(time.Second))
			}
		} else {
			select {
			case <-wakeUp:
				sm.emitTestingEvent(ctx, "woken")
			case <-ctx.Done():
			}
		}
	}
}

// RandomSecret returns a random secret given its name.
func (sm *SecretManagerStore) RandomSecret(ctx context.Context, name string) (Secret, error) {
	if sm.randomSecrets == nil {
		return Secret{}, errors.New("random secrets store is not initialized")
	}
	return sm.randomSecrets.RandomSecret(ctx, name)
}

// StoredSecret returns a stored secret given its name.
//
// Value of `name` should have form:
//   - `sm://<project>/<secret>`: a concrete secret in Google Secret Manager.
//   - `sm://<secret>`: same as `sm://<CloudProject>/<secret>`.
//   - `devsecret://<base64-encoded secret>`: return this concrete secret.
//   - `devsecret-gen://tink/aead`: generate a new secret of the Tink AEAD.
//   - `devsecret-text://<string>`: return this concrete secret.
//
// Caches secrets loaded from Google Secret Manager in memory and sets up
// a periodic background task to update the cached values to facilitate graceful
// rotation.
//
// Calls to StoredSecret return the latest value from this local cache and thus
// are fast. To be notified about changes to the secret as soon as they are
// detected use AddRotationHandler.
func (sm *SecretManagerStore) StoredSecret(ctx context.Context, name string) (Secret, error) {
	name, err := sm.normalizeName(name)
	if err != nil {
		return Secret{}, err
	}

	sm.rwm.RLock()
	known := sm.secretsByName[name]
	sm.rwm.RUnlock()
	if known != nil {
		return known.value, nil
	}

	// Note: this lock effectively means we load one secret at a time. This should
	// be fine, there shouldn't be many secrets. And it is probably better to
	// serialize all loading than to hit the GSM from a lot of handlers at the
	// same time when referring to a "popular" secret.
	sm.rwm.Lock()
	defer sm.rwm.Unlock()

	// Double check after grabbing the write lock.
	if known := sm.secretsByName[name]; known != nil {
		return known.value, nil
	}

	// Read the initial values of the secret.
	secret, err := sm.readSecret(ctx, name)
	if err != nil {
		return Secret{}, err
	}
	secret.logActiveVersions(ctx)
	secret.logNextReloadTime(ctx)

	if sm.secretsByName == nil {
		sm.secretsByName = make(map[string]*trackedSecret, 1)
	}
	sm.secretsByName[name] = secret

	// Wake up the MaintenanceLoop (if any) to let it reschedule the next refresh.
	if !secret.nextReload.IsZero() {
		heap.Push(&sm.secretsByTime, secret)
		if sm.wakeUp != nil {
			close(sm.wakeUp)
			sm.wakeUp = nil
		}
	}

	return secret.value, nil
}

// AddRotationHandler registers a callback which is called when the stored
// secret is updated.
//
// The handler is called from an internal goroutine and receives a context
// passed to MaintenanceLoop. If multiple handlers for the same secret are
// registered, they are called in order of their registration one by one.
func (sm *SecretManagerStore) AddRotationHandler(ctx context.Context, name string, cb RotationHandler) error {
	switch name, err := sm.normalizeName(name); {
	case err != nil:
		return err

	case !strings.HasPrefix(name, "sm://"):
		return nil // no updates for static secrets

	default:
		sm.rwm.Lock()
		defer sm.rwm.Unlock()
		if sm.handlers == nil {
			sm.handlers = make(map[string][]RotationHandler, 1)
		}
		sm.handlers[name] = append(sm.handlers[name], cb)
		return nil
	}
}

// ReportMetrics is called on each metrics flush to populate metrics.
func (sm *SecretManagerStore) ReportMetrics(ctx context.Context) {
	sm.rwm.RLock()
	defer sm.rwm.RUnlock()
	for name, secret := range sm.secretsByName {
		// `name` is output of normalizeName(...)
		if strings.HasPrefix(name, "sm://") {
			// `name` is "sm://<project>/<secret>".
			parts := strings.SplitN(name[len("sm://"):], "/", 2)
			project, name := parts[0], parts[1]
			versionMetric.Set(ctx, secret.versionCurrent, project, name, "current")
			versionMetric.Set(ctx, secret.versionPrevious, project, name, "previous")
			versionMetric.Set(ctx, secret.versionNext, project, name, "next")
		}
	}
}

////////////////////////////////////////////////////////////////////////////////

const (
	// Randomized secret reloading interval. It is pretty big, since secrets are
	// assumed to be rotated infrequently. In rare emergencies a service can be
	// restarted to pick new secrets faster.
	reloadIntervalMin = 2 * time.Hour
	reloadIntervalMax = 4 * time.Hour

	// Max delay when retrying failing fetches.
	maxRetryDelay = 30 * time.Minute
)

// trackedSecret is a secret that is periodically reread in MaintenanceLoop.
//
// Instances of this type are static once constructed and thus are safe to
// share across goroutines.
type trackedSecret struct {
	name  string // the name it was loaded under
	value Secret // the latest fetched state of the secret

	versionCurrent  int64 // the currently active version or 0 for static dev secrets
	versionPrevious int64 // the previously active version or 0 if not available
	versionNext     int64 // the next active version or 0 if not available

	attempts   int       // how many consecutive times we failed to reload the secret
	nextReload time.Time // when we should reload the secret or zero for static dev secrets
}

func (s *trackedSecret) logActiveVersions(ctx context.Context) {
	if s.versionCurrent == 0 {
		return
	}

	formatVer := func(v int64) string {
		if v == 0 {
			return "none"
		}
		return strconv.FormatInt(v, 10)
	}

	logging.Infof(ctx, "Loaded secret %q (versions: current=%s, previous=%s, next=%s)",
		s.name,
		formatVer(s.versionCurrent),
		formatVer(s.versionPrevious),
		formatVer(s.versionNext),
	)
}

func (s *trackedSecret) logNextReloadTime(ctx context.Context) {
	if !s.nextReload.IsZero() {
		logging.Debugf(ctx, "Will attempt to reload the secret %q in %s", s.name, s.nextReload.Sub(clock.Now(ctx)))
	}
}

type trackedSecretsPQ []*trackedSecret

func (pq trackedSecretsPQ) Len() int           { return len(pq) }
func (pq trackedSecretsPQ) Less(i, j int) bool { return pq[i].nextReload.Before(pq[j].nextReload) }
func (pq trackedSecretsPQ) Swap(i, j int)      { pq[i], pq[j] = pq[j], pq[i] }

func (pq *trackedSecretsPQ) Push(x any) {
	*pq = append(*pq, x.(*trackedSecret))
}

func (pq *trackedSecretsPQ) Pop() any {
	panic("Pop is not actually used, but defined to comply with heap.Interface")
}

// normalizeName check the secret name format and normalizes it.
func (sm *SecretManagerStore) normalizeName(name string) (string, error) {
	switch {
	case strings.HasPrefix(name, "devsecret://"):
		return name, nil

	case strings.HasPrefix(name, "devsecret-gen://"):
		return name, nil

	case strings.HasPrefix(name, "devsecret-text://"):
		return name, nil

	case strings.HasPrefix(name, "sm://"):
		switch parts := strings.Split(strings.TrimPrefix(name, "sm://"), "/"); {
		case len(parts) == 1:
			if sm.CloudProject == "" {
				return "", errors.Fmt("can't use secret reference %q when the Cloud Project name is not configured", name)
			}
			return fmt.Sprintf("sm://%s/%s", sm.CloudProject, parts[0]), nil
		case len(parts) == 2:
			return name, nil
		default:
			return "", errors.New("sm:// secret reference should have form sm://<name> or sm://<project>/<name>")
		}

	default:
		return "", errors.Fmt("not supported secret reference %q", name)
	}
}

// readSecret fetches a secret given its normalized name.
func (sm *SecretManagerStore) readSecret(ctx context.Context, name string) (*trackedSecret, error) {
	switch {
	case strings.HasPrefix(name, "devsecret://"):
		value, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(name, "devsecret://"))
		if err != nil {
			return nil, errors.Fmt("bad devsecret://, not base64 encoding: %w", err)
		}
		return &trackedSecret{
			name:  name,
			value: Secret{Active: value},
		}, nil

	case strings.HasPrefix(name, "devsecret-gen://"):
		switch kind := strings.TrimPrefix(name, "devsecret-gen://"); kind {
		case "tink/aead":
			value, err := generateDevTinkAEADKeyset(ctx)
			if err != nil {
				return nil, errors.Fmt("failed to generate new tink AEAD keyset: %w", err)
			}
			return &trackedSecret{
				name:  name,
				value: Secret{Active: value},
			}, nil
		default:
			return nil, errors.Fmt("devsecret-gen:// kind %q is not supported", kind)
		}

	case strings.HasPrefix(name, "devsecret-text://"):
		return &trackedSecret{
			name:  name,
			value: Secret{Active: []byte(strings.TrimPrefix(name, "devsecret-text://"))},
		}, nil

	case strings.HasPrefix(name, "sm://"):
		return sm.readSecretFromGSM(ctx, name)

	default:
		panic("impossible, already checked in normalizeSecretName")
	}
}

// readSecretFromGSM returns a sm://... secret given its normalized name.
func (sm *SecretManagerStore) readSecretFromGSM(ctx context.Context, name string) (*trackedSecret, error) {
	logging.Debugf(ctx, "Loading secret %q", name)

	// `name` here must have sm://<project>/<secret> format.
	parts := strings.Split(strings.TrimPrefix(name, "sm://"), "/")
	if len(parts) != 2 {
		panic("impossible, should be normalize already")
	}
	project, secret := parts[0], parts[1]

	// Try to access "current", "previous", "next" versions if they are available.
	eg, egctx := errgroup.WithContext(ctx)
	var (
		current  secretVersion
		previous secretVersion
		next     secretVersion
	)
	attemptAccess := func(ver string, dest *secretVersion) error {
		switch resp, err := sm.accessSecretVersion(egctx, project, secret, ver); {
		case err == nil:
			*dest = *resp
		case !isMissingOrDisabledVersion(err):
			return errors.Fmt("version %q: %w", ver, err)
		}
		return nil
	}
	eg.Go(func() error { return attemptAccess("current", &current) })
	eg.Go(func() error { return attemptAccess("previous", &previous) })
	eg.Go(func() error { return attemptAccess("next", &next) })
	if err := eg.Wait(); err != nil {
		return nil, errors.Fmt("loading the secret %q: %w", name, err)
	}

	// If there's no "current", fallback to loading "latest" instead of "current"
	// and the one version before it as instead of "previous" (if necessary). This
	// is a scheme used by this code previously.
	if current.version == 0 {
		latest, err := sm.accessSecretVersion(ctx, project, secret, "latest")
		if err != nil {
			return nil, errors.Fmt("failed to get the latest version of the secret %q: %w", name, err)
		}
		current = *latest
		if current.version > 1 && previous.version == 0 {
			prev, err := sm.accessSecretVersion(ctx, project, secret, fmt.Sprintf("%d", current.version-1))
			switch {
			case err == nil:
				previous = *prev
			case !isMissingOrDisabledVersion(err):
				return nil, errors.Fmt("failed to get the previous version of the secret %q: %w", name, err)
			}
		}
	}

	// Set the active version, collect non-current versions into "Passive" list.
	value := Secret{Active: current.payload}
	seen := map[int64]bool{current.version: true}
	if previous.version != 0 && !seen[previous.version] {
		value.Passive = append(value.Passive, previous.payload)
		seen[previous.version] = true
	}
	if next.version != 0 && !seen[next.version] {
		value.Passive = append(value.Passive, next.payload)
		seen[next.version] = true
	}

	return &trackedSecret{
		name:            name,
		value:           value,
		versionCurrent:  current.version,
		versionPrevious: previous.version, // may be 0
		versionNext:     next.version,     // may be 0
		nextReload:      nextReloadTime(ctx),
	}, nil
}

type secretVersion struct {
	version int64  // integer version, >=1 for loaded secrets
	payload []byte // the actual secret value
}

// accessSecretVersion wraps the AccessSecretVersion RPC, decoding the version.
//
// Returns gRPC errors.
func (sm *SecretManagerStore) accessSecretVersion(ctx context.Context, project, secret, version string) (*secretVersion, error) {
	resp, err := sm.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, secret, version),
	})
	if err != nil {
		return nil, err
	}
	// The version name has format "projects/.../secrets/.../versions/<number>".
	// We want to grab the version number. Note that GSM uses numeric project IDs
	// in `resp.Name` instead of Cloud Project names, so we can't just trim
	// the prefix.
	idx := strings.LastIndex(resp.Name, "/")
	if idx == -1 {
		return nil, status.Errorf(codes.Unknown, "unexpected version name format %q", resp.Name)
	}
	ver, err := strconv.ParseInt(resp.Name[idx+1:], 10, 64)
	if err != nil {
		return nil, status.Errorf(codes.Unknown, "unexpected version name format %q", resp.Name)
	}
	if ver <= 0 {
		return nil, status.Errorf(codes.Unknown, "the version is unexpectedly non-positive %q", resp.Name)
	}
	return &secretVersion{
		version: ver,
		payload: resp.Payload.Data,
	}, nil
}

// reloadNextSecretLocked looks at the secret at the top of secretsByTime PQ and
// reloads it.
//
// Returns false if the top of the queue is not due for the reload yet.
//
// Launches RotationHandler in individual goroutines adding them to `wg`.
func (sm *SecretManagerStore) reloadNextSecretLocked(ctx context.Context, wg *sync.WaitGroup) bool {
	if len(sm.secretsByTime) == 0 || clock.Now(ctx).Before(sm.secretsByTime[0].nextReload) {
		return false
	}

	// Reload the secret. This always changes its nextReload, even on failures.
	secret := sm.secretsByTime[0]
	updated := sm.tryReloadSecretLocked(ctx, secret)
	heap.Fix(&sm.secretsByTime, 0) // fix after its nextReload changed

	// Call RotationHandler callbacks from a goroutine to avoid blocking the loop.
	if updated {
		handlers := append([]RotationHandler(nil), sm.handlers[secret.name]...)
		newValue := secret.value
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, cb := range handlers {
				cb(ctx, newValue)
			}
		}()
		sm.emitTestingEvent(ctx, "reloaded %s", secret.name)
	} else {
		sm.emitTestingEvent(ctx, "checked %s", secret.name)
	}

	return true
}

// tryReloadSecretLocked attempts to reload the secret, mutating it in-place.
//
// Returns true if the secret has a new value now or false if the value didn't
// change (either we failed to fetch it or it really didn't change).
//
// Logs errors inside. Fields `secret.attempts` and `secret.nextReload` are
// mutated even on failures.
func (sm *SecretManagerStore) tryReloadSecretLocked(ctx context.Context, secret *trackedSecret) bool {
	fresh, err := sm.readSecret(ctx, secret.name)
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

// emitTestingEvent is used in tests to expose what MaintenanceLoop is doing.
func (sm *SecretManagerStore) emitTestingEvent(ctx context.Context, msg string, args ...any) {
	if sm.testingEvents != nil {
		select {
		case sm.testingEvents <- fmt.Sprintf(msg, args...):
		case <-ctx.Done():
		}
	}
}

// isMissingOrDisabledVersion checks for the gRPC code representing missing or
// disabled versions.
func isMissingOrDisabledVersion(err error) bool {
	code := status.Code(err)
	return code == codes.NotFound || code == codes.FailedPrecondition
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

// generateDevTinkAEADKeyset generates "devsecret-gen://tink/aead" key.
func generateDevTinkAEADKeyset(ctx context.Context) ([]byte, error) {
	kh, err := keyset.NewHandle(aead.AES256GCMKeyTemplate())
	if err != nil {
		return nil, errors.Fmt("failed to generate from template: %w", err)
	}
	buf := bytes.NewBuffer(nil)
	if err := insecurecleartextkeyset.Write(kh, keyset.NewJSONWriter(buf)); err != nil {
		return nil, errors.Fmt("failed to serialize newly generated keyset: %w", err)
	}
	value := buf.Bytes()
	logging.Infof(
		ctx,
		"Generated a new development AEAD keyset. To re-use locally during development, "+
			"replace \"devsecret-gen://tink/aead\" with the value below\n\tdevsecret://%s",
		base64.RawStdEncoding.EncodeToString(value),
	)
	return value, nil
}
