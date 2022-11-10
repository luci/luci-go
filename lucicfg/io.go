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

package lucicfg

import (
	"go.starlark.net/starlark"

	"go.chromium.org/luci/starlark/interpreter"
)

// How many times a file needs to be read before its data is cached in memory.
//
// E.g. if cachingThreshold is 2, first two io.read_file calls will actually
// load the file from disk, and only the third call (and onward) will use
// the cached data.
//
// This is a heuristic to avoid spending RAM on files that will be read only
// once.
const cachingThreshold = 2

type fileCache struct {
	interner stringInterner
	cache    map[interpreter.ModuleKey]fileCacheEntry
}

type fileCacheEntry struct {
	hits int    // 0, 1, .., cachingThreshold (no counting after cachingThreshold)
	data string // the cached data if hits == cachingThreshold, "" otherwise
}

func newFileCache(interner stringInterner) fileCache {
	return fileCache{
		interner: interner,
		cache:    map[interpreter.ModuleKey]fileCacheEntry{},
	}
}

func (c *fileCache) get(key interpreter.ModuleKey, load func() (string, error)) (string, error) {
	entry := c.cache[key]
	if entry.hits == cachingThreshold {
		return entry.data, nil
	}

	data, err := load()
	if err != nil {
		return "", err
	}

	if data != "" {
		entry.hits += 1
		if entry.hits == cachingThreshold {
			entry.data = c.interner.internString(data)
		}
	} else {
		// Can cache "" right away, no RAM impact.
		entry.hits = cachingThreshold
	}

	c.cache[key] = entry
	return data, nil
}

func init() {
	// See //internal/io.star.
	declNative("read_file", func(call nativeCall) (starlark.Value, error) {
		var path starlark.String
		if err := call.unpack(1, &path); err != nil {
			return nil, err
		}
		intr := interpreter.GetThreadInterpreter(call.Thread)
		key, err := interpreter.MakeModuleKey(call.Thread, path.GoString())
		if err != nil {
			return nil, err
		}
		src, err := call.State.files.get(key, func() (string, error) {
			return intr.LoadSource(call.Thread, path.GoString())
		})
		return starlark.String(src), err
	})
}
