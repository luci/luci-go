// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
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

	// CacheFilename is a name of the file with all cached tokens.
	CacheFilename = "creds.json"
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

type cacheFileEntry struct {
	Key        CacheKey     `json:"key"`
	Token      oauth2.Token `json:"token"`
	LastUpdate time.Time    `json:"last_update"`
}

func (e *cacheFileEntry) isOld(now time.Time) bool {
	delay := GCAccessTokenMaxAge
	if e.Token.RefreshToken != "" {
		delay = GCRefreshTokenMaxAge
	}
	exp := e.Token.Expiry
	if exp.IsZero() {
		exp = e.LastUpdate
	}
	return now.Sub(exp) >= delay
}

func (c *DiskTokenCache) absPath() string {
	return filepath.Join(c.SecretsDir, CacheFilename)
}

// readCacheFile loads the file with cached tokens.
func (c *DiskTokenCache) readCacheFile() (*cacheFile, error) {
	// Minimize the time the file is locked on Windows by reading it all at once
	// and decoding later.
	//
	// We also need to open it with FILE_SHARE_DELETE sharing mode to allow
	// writeCacheFile() below to replace open files (even though it tries to wait
	// for the file to be closed). For some reason, omitting FILE_SHARE_DELETE
	// flag causes random sharing violation errors when opening the file for
	// reading.
	f, err := openSharedDelete(c.absPath())
	switch {
	case os.IsNotExist(err):
		return &cacheFile{}, nil
	case err != nil:
		return nil, err
	}
	blob, err := ioutil.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, err
	}

	cache := &cacheFile{}
	if err := json.Unmarshal(blob, cache); err != nil {
		// If the cache file got broken somehow, it makes sense to treat it as
		// empty (so it can later be overwritten), since it's unlikely it's going
		// to "fix itself".
		logging.WithError(err).Warningf(c.Context, "The token cache %s is broken", c.absPath())
		return &cacheFile{}, nil
	}

	return cache, nil
}

// writeCacheFile overwrites the file with cached tokens.
//
// It also automatically discards old entries.
//
// Returns a transient error if the file is locked by some other process and
// can't be updated (this happens on Windows).
func (c *DiskTokenCache) writeCacheFile(cache *cacheFile, now time.Time) error {
	// Throw away old entries.
	cleaned := *cache
	cleaned.Cache = make([]*cacheFileEntry, 0, len(cache.Cache))
	cleaned.LastUpdate = now
	for _, entry := range cache.Cache {
		if entry.isOld(now) {
			logging.Debugf(c.Context, "Cleaning up old token cache entry: %s", entry.Key.Key)
		} else {
			cleaned.Cache = append(cleaned.Cache, entry)
		}
	}

	// Nothing left? Remove the file completely.
	if len(cleaned.Cache) == 0 {
		if err := os.Remove(c.absPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	blob, err := json.MarshalIndent(cleaned, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first.
	if err := os.MkdirAll(c.SecretsDir, 0700); err != nil {
		logging.WithError(err).Warningf(c.Context, "Failed to mkdir token cache dir")
		// carry on, TempFile will fail too.
	}
	tmp, err := ioutil.TempFile(c.SecretsDir, CacheFilename)
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
	if err = os.Rename(tmp.Name(), c.absPath()); err != nil {
		cleanup()
		return errors.WrapTransient(err)
	}
	return nil
}

// updateCacheFile reads the token cache file, calls the callback, writes the file
// back if the callback returns 'true'.
//
// It retries a bunch of times when encountering sharing violation errors on
// Windows.
//
// TODO(vadimsh): Change this to use file locking - updateCacheFile is a global
// critical section.
func (c *DiskTokenCache) updateCacheFile(cb func(*cacheFile, time.Time) bool) error {
	retryParams := func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:    10 * time.Millisecond,
				Retries:  20,
				MaxTotal: 2 * time.Second,
			},
			Multiplier: 1.5,
		}
	}
	return retry.Retry(c.Context, retry.TransientOnly(retryParams), func() error {
		cache, err := c.readCacheFile()
		if err != nil {
			return err
		}
		now := clock.Now(c.Context).UTC()
		if cb(cache, now) {
			return c.writeCacheFile(cache, now)
		}
		return nil
	}, func(err error, d time.Duration) {
		logging.WithError(err).Warningf(c.Context, "Failed to update %s, retrying...", c.absPath())
	})
}

// GetToken reads the token from cache.
func (c *DiskTokenCache) GetToken(key *CacheKey) (*oauth2.Token, error) {
	cache, err := c.readCacheFile()
	if err != nil {
		return nil, err
	}
	for _, entry := range cache.Cache {
		if EqualCacheKeys(&entry.Key, key) {
			return &entry.Token, nil
		}
	}
	return nil, nil
}

// PutToken writes the token to cache.
func (c *DiskTokenCache) PutToken(key *CacheKey, tok *oauth2.Token) error {
	token := *tok
	if !token.Expiry.IsZero() {
		token.Expiry = token.Expiry.UTC()
	}
	return c.updateCacheFile(func(cache *cacheFile, now time.Time) bool {
		for _, entry := range cache.Cache {
			if EqualCacheKeys(&entry.Key, key) {
				entry.Token = token
				entry.LastUpdate = now
				return true
			}
		}
		cache.Cache = append(cache.Cache, &cacheFileEntry{
			Key:        *key,
			Token:      token,
			LastUpdate: now,
		})
		return true
	})
}

// DeleteToken removes the token from cache.
func (c *DiskTokenCache) DeleteToken(key *CacheKey) error {
	return c.updateCacheFile(func(cache *cacheFile, now time.Time) bool {
		for i, entry := range cache.Cache {
			if EqualCacheKeys(&entry.Key, key) {
				cache.Cache = append(cache.Cache[:i], cache.Cache[i+1:]...)
				return true
			}
		}
		return false // not there, this is fine, skip writing the file
	})
}
