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

	. "github.com/smartystreets/goconvey/convey"
)

func TestCache(t *testing.T) {
	t.Parallel()

	Convey("basic", t, func() {
		path := filepath.Join(t.TempDir(), "db")
		k, err := New(context.Background(), path)
		So(err, ShouldBeNil)

		So(k.Set("key1", []byte("value1")), ShouldBeNil)
		So(k.Set("key2", []byte("value2")), ShouldBeNil)
		So(k.Set("key3", []byte("value3")), ShouldBeNil)

		var mu sync.Mutex
		var keys []string
		var values []string

		k.GetMulti([]string{"key1", "key2", "key4"}, func(key string, value []byte) error {
			mu.Lock()
			keys = append(keys, key)
			values = append(values, string(value))
			mu.Unlock()
			return nil
		})

		sort.Strings(keys)
		sort.Strings(values)

		So(keys, ShouldResemble, []string{"key1", "key2"})
		So(values, ShouldResemble, []string{"value1", "value2"})

		keys = nil
		values = nil
		k.ForEach(func(key string, value []byte) error {
			keys = append(keys, key)
			values = append(values, string(value))
			return nil
		})

		So(keys, ShouldResemble, []string{"key1", "key2", "key3"})
		So(values, ShouldResemble, []string{"value1", "value2", "value3"})

		So(k.Close(), ShouldBeNil)
	})
}
