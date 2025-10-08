// Copyright 2025 The LUCI Authors.
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

package gerrit

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/git/creds"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/system/oscompat"
)

type ReAuthCheckResult struct {
	Host    string `json:"host"`    // Gerrit host for the result
	Project string `json:"project"` // Project/repo for the result

	// Whether a RAPT is needed to upload to this project/ref.
	NeedsRAPT bool `json:"needsRapt"`

	// Whether the request included a valid RAPT.
	HasValidRAPT bool `json:"hasValidRapt,omitempty"`
}

// ErrResultMissing is returned if requested result is missing.
var ErrResultMissing = errors.New("Result missing from cache")

// A ReAuthResultCache caches [ReAuthCheckResult] for [ReAuthChecker].
//
// Expiry details are delegated to the implementation.
type ReAuthResultCache interface {
	// Put puts a [ReAuthCheckResult] into the cache.
	// The caller retains ownership of the result object after
	// this method returns.
	Put(context.Context, *ReAuthCheckResult) error

	// GetForProject gets a [ReAuthCheckResult] for the Gerrit project
	// if available in the cache.  The caller has ownership of the
	// returned result.
	//
	// Long term cache implementations should clear short lived
	// fields like HasValidRAPT to prevent client misuse.
	//
	// Returns [ErrResultMissing] if the result is not cached.
	GetForProject(ctx context.Context, host, project string) (*ReAuthCheckResult, error)
}

// A ReAuthChecker checks whether ReAuth is needed for a Gerrit repo.
//
// This keeps a cache as ReAuth requirements don't change often.
type ReAuthChecker struct {
	client *http.Client
	cache  ReAuthResultCache
}

// NewReAuthChecker returns a [ReAuthChecker].
//
// The HTTP client should be authenticated with OAuth credentials with
// Gerrit and ReAuth scopes.  It is used to call the Gerrit ReAuth check API.
func NewReAuthChecker(client *http.Client, cache ReAuthResultCache) *ReAuthChecker {
	if cache == nil {
		panic("nil result cache passed")
	}
	return &ReAuthChecker{
		client: client,
		cache:  cache,
	}
}

// Check checks whether the user needs ReAuth for a Gerrit repo.
//
// Note that the HasValidRAPT field will always be set to false
// because cached values are meaningless.
func (c *ReAuthChecker) Check(ctx context.Context, attrs *creds.Attrs) (*ReAuthCheckResult, error) {
	host := attrs.Host
	project := attrs.Path
	if host == "" {
		return nil, errors.Fmt("check Gerrit ReAuth: empty host not allowed")
	}
	if project == "" {
		return nil, errors.Fmt("check Gerrit ReAuth: empty project not allowed")
	}
	if !isSupportedHost(host) {
		// Unsupported hosts never need ReAuth.
		logging.Debugf(ctx, "Skipping ReAuth check for unsupported host %q", host)
		return &ReAuthCheckResult{
			Host:    host,
			Project: project,
		}, nil
	}
	r, err := c.cache.GetForProject(ctx, host, project)
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, ErrResultMissing) {
		return nil, errors.Fmt("check Gerrit ReAuth: %w", err)
	}
	r, err = checkProjectReAuth(ctx, c.client, host, project)
	if err != nil {
		return nil, errors.Fmt("check Gerrit ReAuth: %w", err)
	}
	if err := c.cache.Put(ctx, r); err != nil {
		logging.Warningf(ctx, "Error caching ReAuth check result: %s", err)
	}
	return r, nil
}

// reauthCapableHosts are Gerrit hosts that supports ReAuth.
//
// Hosts here are without -review or domain suffix.
var reauthCapableHosts = []string{
	"chromium",
	"dawn",
	"webrtc",
}

// Gerrit host suffixes.
var gerritHostSuffixes = []string{
	".googlesource.com",
	"-review.googlesource.com",
}

// isSupportedHost checks if the given host is a supported Gerrit host for ReAuth.
//
// `host` is the host part of a URL (e.g. `chromium.googlesource.com`).
func isSupportedHost(host string) bool {
	for _, prefix := range reauthCapableHosts {
		if remaining, found := strings.CutPrefix(host, prefix); found {
			if slices.Contains(gerritHostSuffixes, remaining) {
				return true
			}
		}
	}
	return false
}

// checkProjectReAuth checks whether the user needs ReAuth for a
// Gerrit project (repo).
//
// The HTTP client should be authenticated with OAuth credentials with
// Gerrit and ReAuth scopes.
//
// If the HTTP client has a RAPT cookie, then the result will indicate
// whether the RAPT is valid.
//
// This does not cache the check result.  Prefer using [ReAuthChecker] which caches.
func checkProjectReAuth(ctx context.Context, c *http.Client, host, project string) (*ReAuthCheckResult, error) {
	res := ReAuthCheckResult{
		Host:    host,
		Project: project,
	}
	logging.Debugf(ctx, "Calling ReAuth check RPC on Gerrit host %q for %q", normalizeGerritHost(host), project)
	gc, err := gerrit.NewRESTClient(c, normalizeGerritHost(host), true)
	if err != nil {
		return nil, err
	}
	resp, err := gc.CheckRapt(ctx, &gerritpb.CheckRaptRequest{Project: res.Project})
	if err != nil {
		return nil, errors.Fmt("checkProjectReAuth on host %q project %q: %w", host, project, err)
	}
	res.NeedsRAPT = resp.GetRaptRequired()
	res.HasValidRAPT = resp.GetHasValidRapt()
	return &res, nil
}

// A MemResultCache caches ReAuth check results in memory.
type MemResultCache struct {
	lock    sync.Mutex
	results []*ReAuthCheckResult
}

func (c *MemResultCache) Put(ctx context.Context, r *ReAuthCheckResult) error {
	copy := *r
	c.lock.Lock()
	c.results = append(c.results, &copy)
	c.lock.Unlock()
	return nil
}

func (c *MemResultCache) GetForProject(ctx context.Context, host, project string) (*ReAuthCheckResult, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	for _, r := range reverse(c.results) {
		if r.Host == host && r.Project == project {
			copy := *r
			return &copy, nil
		}
	}
	return nil, ErrResultMissing
}

// A DiskResultCache caches ReAuth check results on disk.
type DiskResultCache struct {
	cacheDir string
	// This lock simply provides a little protection against
	// concurrent ops in the same process.
	lock sync.Mutex
}

type resultStore struct {
	Entries []*cachedResult `json:"entries"`
}

// cachedResult embeds CheckResult and adds a timestamp.
type cachedResult struct {
	ReAuthCheckResult
	Timestamp time.Time `json:"timestamp"`
	Expiry    time.Time `json:"expiry"`
}

const resultCacheLifetime = time.Hour * 24 * 30

func NewDiskResultCache(ctx context.Context, dir string) *DiskResultCache {
	c := &DiskResultCache{
		cacheDir: dir,
	}
	return c
}

func (c *DiskResultCache) cacheFile() string {
	return filepath.Join(c.cacheDir, "reauth-results-cache.json")
}

func (c *DiskResultCache) createCacheTemp() (*os.File, error) {
	return os.CreateTemp(c.cacheDir, "reauth-results-cache.*.json")
}

func (*DiskResultCache) expired(ctx context.Context, r *cachedResult) bool {
	now := clock.Now(ctx)
	return now.After(r.Expiry)
}

func (c *DiskResultCache) read(ctx context.Context) (*resultStore, error) {
	var s resultStore
	f, err := oscompat.OpenSharedDelete(c.cacheFile())
	if os.IsNotExist(err) {
		// File doesn't exist, start with an empty cache.
		return &s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint: errcheck

	d := json.NewDecoder(f)
	if err := d.Decode(&s); err != nil {
		// Treat corrupt file as empty for transparent recovery.
		logging.Warningf(ctx, "DiskResultCache: Error decoding file: %s", err)
		return &s, nil
	}
	processed := make([]*cachedResult, 0, len(s.Entries))
	for _, r := range s.Entries {
		if !c.expired(ctx, r) {
			processed = append(processed, r)
		}
	}
	s.Entries = processed
	return &s, nil
}

func (c *DiskResultCache) write(ctx context.Context, s *resultStore) (err error) {
	f, err := c.createCacheTemp()
	if err != nil {
		return errors.Fmt("DiskResultCache.write: %w", err)
	}
	defer f.Close() //nolint:errcheck
	moved := false
	defer func() {
		if !moved {
			if err := os.Remove(f.Name()); err != nil {
				logging.Warningf(ctx, "Error removing temp file %q: %s", f.Name(), err)
			}
		}
	}()

	e := json.NewEncoder(f)
	if err := e.Encode(s); err != nil {
		return errors.Fmt("DiskResultCache.write: %w", err)
	}
	if err := f.Close(); err != nil {
		return errors.Fmt("DiskResultCache.write: %w", err)
	}
	if err := os.Rename(f.Name(), c.cacheFile()); err != nil {
		return errors.Fmt("DiskResultCache.write: %w", err)
	}
	moved = true
	return nil
}

func (c *DiskResultCache) GetForProject(ctx context.Context, host, project string) (*ReAuthCheckResult, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	store, err := c.read(ctx)
	if err != nil {
		return nil, errors.Fmt("DiskResultCache.GetForProject: %w", err)
	}
	for _, r := range reverse(store.Entries) {
		if r.Host == host && r.Project == project && !c.expired(ctx, r) {
			copy := r.ReAuthCheckResult
			return &copy, nil
		}
	}
	return nil, ErrResultMissing
}

func (c *DiskResultCache) Put(ctx context.Context, r *ReAuthCheckResult) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	s, err := c.read(ctx)
	if err != nil {
		return errors.Fmt("DiskResultCache.Put: %w", err)
	}
	now := clock.Now(ctx)
	cr := &cachedResult{
		ReAuthCheckResult: *r,
		Timestamp:         now,
		Expiry:            now.Add(resultCacheLifetime).Add(time.Duration(mathrand.Float64(ctx) * float64(7*24*time.Hour))),
	}
	// This value is meaningless when cached since RAPTs have very
	// short lifetimes.
	cr.HasValidRAPT = false
	s.Entries = append(s.Entries, cr)
	if err := c.write(ctx, s); err != nil {
		return errors.Fmt("DiskResultCache.Put: %w", err)
	}
	return nil
}

// Clear clears all entries in the cache.
func (c *DiskResultCache) Clear() error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := os.Remove(c.cacheFile()); err != nil {
		return errors.Fmt("DiskResultCache.Clear: %w", err)
	}
	return nil
}

func reverse[V any](s []V) func(yield func(int, V) bool) {
	return func(yield func(int, V) bool) {
		for i := range s {
			j := len(s) - i - 1
			if !yield(j, s[j]) {
				return
			}
		}
	}
}

// normalizeGerritHost ensures the Gerrit host is the -review kind.
// This is needed to access the check RAPT API.
func normalizeGerritHost(host string) string {
	parts := strings.Split(host, ".")
	if !strings.HasSuffix(parts[0], "-review") {
		parts[0] = parts[0] + "-review"
	}
	return strings.Join(parts, ".")
}
