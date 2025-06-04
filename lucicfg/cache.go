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

package lucicfg

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
)

// CacheEnvVar is the env var name that controls location of the cache
// directory.
const CacheEnvVar = "LUCICFG_CACHE_DIR"

var (
	defaultCacheOnce sync.Once
	defaultCacheDir  string
	defaultCacheErr  error
)

// DefaultCacheDir is default cache directory location.
//
// In extreme cases (e.g. empty environment) may fallback to a new temp
// directory, returning an error if it can't be created.
func DefaultCacheDir() (string, error) {
	defaultCacheOnce.Do(func() {
		if cacheDir, err := os.UserCacheDir(); err == nil {
			defaultCacheDir = filepath.Join(cacheDir, "lucicfg")
		} else {
			var err error
			if defaultCacheDir, err = os.MkdirTemp("", "lucicfg"); err != nil {
				defaultCacheErr = errors.Fmt("creating temp cache directory: %w", err)
			}
		}
	})
	return defaultCacheDir, defaultCacheErr
}

// Cache represents lucicfg cache directory.
//
// Note this directory can be shared by multiple processes and access will be
// synchronized.
type Cache struct {
	envVarVal string // value of CacheEnvVar env var or ""

	once sync.Once // to lazy-initialize guts on first access
	err  error     // non-nil if the cache is fatally broken
	dir  string    // actual cache directory path (created if necessary)
}

// OpenCache initializes a new or reuses an existing cache directory.
//
// Its location is taken from LUCICFG_CACHE_DIR env var in the context, with a
// fallback to the DefaultCacheDir.
func OpenCache(ctx context.Context) *Cache {
	return &Cache{
		envVarVal: environ.FromCtx(ctx).Get(CacheEnvVar),
	}
}

// init does lazy-initialization.
func (c *Cache) init() error {
	c.once.Do(func() {
		if err := c.doInit(); err != nil {
			c.err = errors.Fmt("failed to initialize the cache directory: %w", err)
		}
	})
	return c.err
}

// doInit does actual initialization, called once.
func (c *Cache) doInit() error {
	var err error
	dir := c.envVarVal
	if dir == "" {
		if dir, err = DefaultCacheDir(); err != nil {
			return err
		}
	}
	if !filepath.IsAbs(dir) {
		return errors.Fmt("$%s should be an absolute path, got %q", CacheEnvVar, dir)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	c.dir = dir
	return nil
}

// Subdir creates a cache subdirectory if doesn't exist yet.
//
// Returns an absolute path to it.
func (c *Cache) Subdir(path string) (string, error) {
	if err := c.init(); err != nil {
		return "", err
	}
	p := filepath.Join(c.dir, path)
	if err := os.MkdirAll(p, 0700); err != nil {
		return "", err
	}
	return p, nil
}
