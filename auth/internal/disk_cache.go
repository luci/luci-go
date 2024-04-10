// Copyright 2017 The LUCI Authors.
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

package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

const (
	// GCAccessTokenMaxAge defines when to remove unused access tokens from the
	// disk cache.
	//
	// We define "an access token" as an instance of oauth2.Token with
	// RefreshToken set to "".
	//
	// If an access token expired older than GCAccessTokenMaxAge ago, it will be
	// evicted from the cache (it is essentially garbage now anyway).
	GCAccessTokenMaxAge = 2 * time.Hour

	// GCRefreshTokenMaxAge defines when to remove unused refresh tokens from the
	// disk cache.
	//
	// We define "a refresh token" as an instance of oauth2.Token with
	// RefreshToken not set to "".
	//
	// Refresh tokens don't expire, but they get neglected and forgotten by users,
	// staying on their disks forever. We remove such tokens if they haven't been
	// used for more than two weeks.
	//
	// It essentially logs out the user on inactivity. We don't actively revoke
	// evicted tokens though, since it's possible the user has copied the token
	// and uses it elsewhere (it happens). Such token can always be revoked from
	// Google Accounts page manually, if required.
	GCRefreshTokenMaxAge = 14 * 24 * time.Hour
)

// DiskTokenCache implements TokenCache on top of a file.
//
// It uses single file to store all tokens. If multiple processes try to write
// to it at the same time, only one process wins (so some updates may be lost).
//
// TODO(vadimsh): Either use file locking or split the cache into multiple
// files to avoid concurrency issues.
//
// TODO(vadimsh): Once this implementation settles and is deployed everywhere,
// add a cleanup step that removes <cache_dir>/*.tok left from the previous
// version of this code.
type DiskTokenCache struct {
	Context    context.Context // for logging and timing
	SecretsDir string
}

type cacheFile struct {
	Cache      []*cacheFileEntry `json:"cache"`
	LastUpdate time.Time         `json:"last_update"`
}

// cacheFileEntry holds one set of cached tokens.
//
// Implements custom JSON marshaling logic to round-trip unknown fields. This
// is useful when new fields are added by newer code, but the token cache is
// still used by older code. Extra fields are better left untouched by the older
// code.
type cacheFileEntry struct {
	key        CacheKey
	token      oauth2.Token
	idToken    string
	email      string
	lastUpdate time.Time

	extra map[string]*json.RawMessage
}

type keyPtr struct {
	key string
	ptr any
}

func (e *cacheFileEntry) structure() []keyPtr {
	return []keyPtr{
		{"key", &e.key},
		{"token", &e.token},
		{"id_token", &e.idToken},
		{"email", &e.email},
		{"last_update", &e.lastUpdate},
	}
}

func (e *cacheFileEntry) UnmarshalJSON(data []byte) error {
	*e = cacheFileEntry{extra: make(map[string]*json.RawMessage)}
	if err := json.Unmarshal(data, &e.extra); err != nil {
		return err
	}
	for _, kv := range e.structure() {
		if raw := e.extra[kv.key]; raw != nil {
			delete(e.extra, kv.key)
			if err := json.Unmarshal([]byte(*raw), kv.ptr); err != nil {
				return fmt.Errorf("when JSON decoding %q - %s", kv.key, err)
			}
		}
	}
	return nil
}

func (e *cacheFileEntry) MarshalJSON() ([]byte, error) {
	// Note: this way of marshaling preserves the order of keys per structure().
	// All unrecognized extra keys are placed at the end, sorted.

	fields := e.structure()
	if len(e.extra) != 0 {
		l := len(fields)
		for k, v := range e.extra {
			fields = append(fields, keyPtr{k, v})
		}
		extra := fields[l:]
		sort.Slice(extra, func(i, j int) bool { return extra[i].key < extra[j].key })
	}

	out := bytes.Buffer{}
	out.WriteString("{")

	first := true
	for _, kv := range fields {
		if !first {
			out.WriteString(",")
		}
		first = false
		fmt.Fprintf(&out, "%q:", kv.key)
		if err := json.NewEncoder(&out).Encode(kv.ptr); err != nil {
			return nil, fmt.Errorf("when JSON encoding %q - %s", kv.key, err)
		}
	}

	out.WriteString("}")
	return out.Bytes(), nil
}

func (e *cacheFileEntry) isOld(now time.Time) bool {
	delay := GCAccessTokenMaxAge
	if e.token.RefreshToken != "" {
		delay = GCRefreshTokenMaxAge
	}
	exp := e.token.Expiry
	if exp.IsZero() {
		exp = e.lastUpdate
	}
	return now.Sub(exp.Round(0)) >= delay
}

func (c *DiskTokenCache) legacyPath() string {
	return filepath.Join(c.SecretsDir, "creds.json")
}

func (c *DiskTokenCache) tokensPath() string {
	return filepath.Join(c.SecretsDir, "tokens.json")
}

// readCacheFile loads the file with cached tokens.
func (c *DiskTokenCache) readCacheFile(path string) (*cacheFile, error) {
	// Minimize the time the file is locked on Windows by reading it all at once
	// and decoding later.
	//
	// We also need to open it with FILE_SHARE_DELETE sharing mode to allow
	// writeCacheFile() below to replace open files (even though it tries to wait
	// for the file to be closed). For some reason, omitting FILE_SHARE_DELETE
	// flag causes random sharing violation errors when opening the file for
	// reading.
	f, err := openSharedDelete(path)
	switch {
	case os.IsNotExist(err):
		return &cacheFile{}, nil
	case err != nil:
		return nil, err
	}
	blob, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, err
	}

	cache := &cacheFile{}
	if err := json.Unmarshal(blob, cache); err != nil {
		// If the cache file got broken somehow, it makes sense to treat it as
		// empty (so it can later be overwritten), since it's unlikely it's going
		// to "fix itself".
		logging.Warningf(c.Context, "The token cache %s is broken: %s", path, err)
		return &cacheFile{}, nil
	}

	return cache, nil
}

// writeCacheFile overwrites the file with cached tokens.
//
// Returns a transient error if the file is locked by some other process and
// can't be updated (this happens on Windows).
func (c *DiskTokenCache) writeCacheFile(path string, cache *cacheFile) error {
	// Nothing left? Remove the file completely.
	if len(cache.Cache) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	blob, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first.
	if err := os.MkdirAll(c.SecretsDir, 0700); err != nil {
		logging.WithError(err).Warningf(c.Context, "Failed to mkdir token cache dir")
		// carry on, TempFile will fail too.
	}
	tmp, err := ioutil.TempFile(c.SecretsDir, "tokens.json.*")
	if err != nil {
		return err
	}

	cleanup := func() {
		if err := os.Remove(tmp.Name()); err != nil {
			logging.WithError(err).Warningf(c.Context, "Failed to remove temp creds cache file: %s", tmp.Name())
		}
	}

	_, writeErr := tmp.Write(blob)
	closeErr := tmp.Close()
	switch {
	case writeErr != nil:
		err = writeErr
	case closeErr != nil:
		err = closeErr
	}
	if err != nil {
		cleanup()
		return err
	}

	// Note that TempFile creates the file in 0600 mode already, so we don't need
	// to chmod it.
	//
	// On Windows Rename may fail with sharing violation error if some other
	// process has opened the file. We treat it as transient error, to trigger
	// a retry in updateCacheFile.
	if err = os.Rename(tmp.Name(), path); err != nil {
		cleanup()
		return transient.Tag.Apply(err)
	}
	return nil
}

// updateCache reads token cache files, calls the callback and writes the files
// back if the callback returns 'true'.
//
// It retries a bunch of times when encountering sharing violation errors on
// Windows.
//
// Mutates tokens.json and creds.json. tokens.json is the primary token cache
// file and creds.json is an old one used by the older versions of this library.
// It will eventually be phased out.
//
// TODO(vadimsh): Change this to use file locking - updateCacheFile is a global
// critical section.
func (c *DiskTokenCache) updateCache(cb func(*cacheFile, time.Time) bool) error {
	retryParams := func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:    10 * time.Millisecond,
				Retries:  200,
				MaxTotal: 4 * time.Second,
			},
			Multiplier: 1.5,
		}
	}
	return retry.Retry(c.Context, transient.Only(retryParams), func() error {
		return c.updateCacheFiles(cb)
	}, func(err error, _ time.Duration) {
		logging.Warningf(c.Context, "Retrying the failed token cache update: %s", err)
	})
}

// readCache reads tokens.json and creds.json and merges them.
func (c *DiskTokenCache) readCache() (*cacheFile, time.Time, error) {
	legacyCache, err := c.readCacheFile(c.legacyPath())
	if err != nil {
		return nil, time.Time{}, err
	}
	newCache, err := c.readCacheFile(c.tokensPath())
	if err != nil {
		return nil, time.Time{}, err
	}

	// Merge tokens from legacyCache into newCache, but don't override anything.
	seen := stringset.New(len(newCache.Cache))
	for _, entry := range newCache.Cache {
		seen.Add(entry.key.ToMapKey())
	}
	for _, entry := range legacyCache.Cache {
		if !seen.Has(entry.key.ToMapKey()) {
			newCache.Cache = append(newCache.Cache, entry)
		}
	}

	// If legacyCache didn't exist at all, pretend it was touched in distant past.
	// This avoid weird looking "0001-01-01" dates. Seventies were better.
	if legacyCache.LastUpdate.IsZero() {
		legacyCache.LastUpdate = time.Date(1970, time.January, 01, 0, 0, 0, 0, time.UTC)
	}

	return newCache, legacyCache.LastUpdate, nil
}

// updateCacheFiles does one attempt at updating the cache files.
func (c *DiskTokenCache) updateCacheFiles(cb func(*cacheFile, time.Time) bool) error {
	// Read and merge tokens.json and creds.json.
	cache, legacyLastUpdate, err := c.readCache()
	if err != nil {
		return err
	}

	// Apply the mutation.
	now := clock.Now(c.Context).UTC()
	if !cb(cache, now) {
		return nil
	}

	// Tidy up the cache before saving it.
	c.discardOldEntries(cache, now)

	// HACK: Update creds.json, but do not touch its "last_update" time. That way
	// refresh tokens created by newer `cipd auth-login ...` would still work with
	// older binaries that look at creds.json, but there's still a way to know
	// when creds.json is not actually used (its `last_update` time would be
	// ancient). This will eventually be used to decide if it is safe to delete
	// creds.json.
	cache.LastUpdate = legacyLastUpdate
	if err := c.writeCacheFile(c.legacyPath(), cache); err != nil {
		return err
	}

	// Update tokens.json as usual, updating its `last_update` field.
	cache.LastUpdate = now
	return c.writeCacheFile(c.tokensPath(), cache)
}

// discardOldEntries filters out old entries.
func (c *DiskTokenCache) discardOldEntries(cache *cacheFile, now time.Time) {
	filtered := cache.Cache[:0]
	for _, entry := range cache.Cache {
		if !entry.isOld(now) {
			filtered = append(filtered, entry)
		} else {
			logging.Debugf(c.Context, "Cleaning up old token cache entry: %s", entry.key.Key)
		}
	}
	cache.Cache = filtered
}

// GetToken reads the token from cache.
func (c *DiskTokenCache) GetToken(key *CacheKey) (*Token, error) {
	cache, _, err := c.readCache()
	if err != nil {
		return nil, err
	}
	for _, entry := range cache.Cache {
		if equalCacheKeys(&entry.key, key) {
			return &Token{
				Token:   entry.token,
				IDToken: entry.idToken,
				Email:   entry.email,
			}, nil
		}
	}
	return nil, nil
}

// PutToken writes the token to cache.
func (c *DiskTokenCache) PutToken(key *CacheKey, tok *Token) error {
	token := tok.Token
	if !token.Expiry.IsZero() {
		token.Expiry = token.Expiry.UTC()
	}
	return c.updateCache(func(cache *cacheFile, now time.Time) bool {
		for _, entry := range cache.Cache {
			if equalCacheKeys(&entry.key, key) {
				entry.token = token
				entry.idToken = tok.IDToken
				entry.email = tok.Email
				entry.lastUpdate = now
				return true
			}
		}
		cache.Cache = append(cache.Cache, &cacheFileEntry{
			key:        *key,
			token:      token,
			idToken:    tok.IDToken,
			email:      tok.Email,
			lastUpdate: now,
		})
		return true
	})
}

// DeleteToken removes the token from cache.
func (c *DiskTokenCache) DeleteToken(key *CacheKey) error {
	return c.updateCache(func(cache *cacheFile, now time.Time) bool {
		for i, entry := range cache.Cache {
			if equalCacheKeys(&entry.key, key) {
				cache.Cache = append(cache.Cache[:i], cache.Cache[i+1:]...)
				return true
			}
		}
		return false // not there, this is fine, skip writing the file
	})
}
