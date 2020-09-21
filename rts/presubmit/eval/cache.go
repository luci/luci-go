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

package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
)

// cacheFile is a cache file in JSON format.
// The value is the path to the file.
type cacheFile string

// TryWrite writes data to the cache file atomically.
// On failure, logs the error.
func (f cacheFile) TryWrite(ctx context.Context, data interface{}) {
	if err := f.Write(ctx, data); err != nil {
		logging.Warningf(ctx, "failed to write cache file %s: %s", f, err)
	}
}

// Write writes data to the cache file atomically.
func (f cacheFile) Write(ctx context.Context, data interface{}) error {
	if err := os.MkdirAll(filepath.Dir(string(f)), 0700); err != nil {
		return err
	}

	tempFile := fmt.Sprintf("%s-%d", f, mathrand.Int(ctx))
	file, err := os.Create(tempFile)
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		os.Remove(tempFile)
	}()

	if err := json.NewEncoder(file).Encode(data); err != nil {
		return err
	}

	if err := file.Close(); err != nil {
		return err
	}
	file = nil

	return os.Rename(tempFile, string(f))
}

// TryRead tries to read the cache file into dest.
// On failure, returns false.
func (f cacheFile) TryRead(ctx context.Context, dest interface{}) bool {
	switch err := f.Read(dest); {
	case os.IsNotExist(err):
		return false

	case err != nil:
		logging.Warningf(ctx, "failed to read cache from %s: %s", string(f), err)
		return false

	default:
		return true
	}
}

// TryRead tries to read the cache file into dest.
func (f cacheFile) Read(dest interface{}) error {
	file, err := os.Open(string(f))
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewDecoder(file).Decode(dest)
}

// cache is a layered key-value cache. A value must be JSON-serializable.
// The first layer is in-memory LRU and the second layer is the file
// system, using cacheFile.
type cache struct {
	dir        string
	memory     *lru.Cache
	structType reflect.Type
}

type maker func() (interface{}, error)

func (c *cache) getOrCreate(ctx context.Context, key string, f maker) (interface{}, error) {
	// The primary motivation of using lru package here is to avoid concurrently
	// calling f for the same key, e.g. to avoid fetching the CL twice.
	return c.memory.GetOrCreate(ctx, key, func() (v interface{}, exp time.Duration, err error) {
		cached := reflect.New(c.structType).Interface()
		file := c.file(key)
		if file.TryRead(ctx, cached) {
			v = cached
			return
		}

		if v, err = f(); err != nil {
			return
		}

		file.TryWrite(ctx, v)
		return
	})
}

func (c *cache) put(ctx context.Context, key string, value interface{}) {
	c.memory.Put(ctx, key, value, 0)
	c.file(key).TryWrite(ctx, value)
}

func (c *cache) file(key string) cacheFile {
	return cacheFile(filepath.Join(c.dir, key))
}
