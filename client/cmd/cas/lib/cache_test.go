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

package lib

import (
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/smartystreets/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCache(t *testing.T) {
	t.Parallel()

	Convey("basic", t, func() {
		path := filepath.Join(t.TempDir(), "db")
		c, err := newbboltCache(path, 1024*1024)
		So(err, ShouldBeNil)

		So(c.put([]byte("key1"), []byte("value1")), ShouldBeNil)
		So(c.put([]byte("key2"), []byte("value2")), ShouldBeNil)
		So(c.put([]byte("key3"), []byte("value3")), ShouldBeNil)

		var mu sync.Mutex
		var keys []string
		var values []string

		c.getMulti([][]byte{[]byte("key1"), []byte("key2"), []byte("key4")}, func(key, value []byte) error {
			mu.Lock()
			keys = append(keys, string(key))
			values = append(values, string(value))
			mu.Unlock()
			return nil
		})

		sort.Strings(keys)
		sort.Strings(values)

		So(keys, ShouldResemble, []string{"key1", "key2"})
		So(values, ShouldResemble, []string{"value1", "value2"})

		So(c.close(), ShouldBeNil)
		_, err = os.Stat(path)
		So(err, ShouldBeNil)
	})
}

func TestCacheClose(t *testing.T) {
	t.Parallel()

	Convey("remove db file", t, func() {
		path := filepath.Join(t.TempDir(), "db")
		c, err := newbboltCache(path, 1)
		So(err, ShouldBeNil)

		So(c.put([]byte("key1"), []byte("value1")), ShouldBeNil)
		So(c.close(), ShouldBeNil)

		_, err = os.Stat(path)
		So(err, assertions.ShouldWrap, os.ErrNotExist)
	})
}
