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

package embeddedkvs

import (
	"context"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCache(t *testing.T) {
	t.Parallel()

	ftt.Run("basic", t, func(t *ftt.Test) {
		path := filepath.Join(t.TempDir(), "db")
		k, err := New(context.Background(), path)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, k.Set("key1", []byte("value1")), should.BeNil)

		assert.Loosely(t, k.SetMulti(func(set func(key string, value []byte) error) error {
			assert.Loosely(t, set("key2", []byte("value2")), should.BeNil)
			assert.Loosely(t, set("key3", []byte("value3")), should.BeNil)
			return nil
		}), should.BeNil)

		var mu sync.Mutex
		var keys []string
		var values []string

		assert.Loosely(t, k.GetMulti(context.Background(), []string{"key1", "key2", "key4"}, func(key string, value []byte) error {
			mu.Lock()
			keys = append(keys, key)
			values = append(values, string(value))
			mu.Unlock()
			return nil
		}), should.BeNil)

		sort.Strings(keys)
		sort.Strings(values)

		assert.Loosely(t, keys, should.Match([]string{"key1", "key2"}))
		assert.Loosely(t, values, should.Match([]string{"value1", "value2"}))

		keys = nil
		values = nil
		k.ForEach(func(key string, value []byte) error {
			keys = append(keys, key)
			values = append(values, string(value))
			return nil
		})

		assert.Loosely(t, keys, should.Match([]string{"key1", "key2", "key3"}))
		assert.Loosely(t, values, should.Match([]string{"value1", "value2", "value3"}))

		assert.Loosely(t, k.Close(), should.BeNil)
	})
}
