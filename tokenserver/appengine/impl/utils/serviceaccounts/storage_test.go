// Copyright 2018 The LUCI Authors.
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

package projectscope

import (
	"context"
	"go.chromium.org/luci/server/caching"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

var lruhandle caching.LRUHandle
func init() {
	lruhandle = caching.RegisterLRUCache(512)
}

type testScopedServiceAccountStorage struct {
	storage [][]string
	onFound func(service, project, override, key string)
	onNotFound func(service, project, override, key string)
}

func (s *testScopedServiceAccountStorage) GetOrCreateIdentity(c context.Context, service, project, override string) (string, error) {
	key, err := getKey(service, project, override)
	if err != nil {
		return "", err
	}

	for i:=0; i<len(s.storage); i++ {
		if s.storage[i][0] == service && s.storage[i][1] == project {
			s.onFound(service, project, override, key)
			return s.storage[i][2], nil
		}
	}

	s.onNotFound(service, project, override, key)
	s.storage = append(s.storage, []string{service, project, key})
	return key, nil
}

func TestScopedServiceAccountStorage(t *testing.T) {
	Convey("Test cached account storage", t, func(){

		ctx := caching.WithEmptyProcessCache(context.Background())

		var found bool = false
		var called bool = false

		mockStorage := &testScopedServiceAccountStorage{
			storage: make([][]string, 0),
			onFound: func (service, project, override, key string) {
				found = true
				called = true
			},
			onNotFound: func(service, project, override, key string) {
				found = false
				called = true
			},
		}

		storage := &cachedIdentityManagerProxy{
			cache:   lruhandle,
			storage: mockStorage,
		}


		storage.cache.LRU(ctx).Put(ctx, "test", "value", time.Minute * 5)
		val, found := storage.cache.LRU(ctx).Get(ctx, "test")
		So(found, ShouldResemble, true)
		So(val, ShouldResemble, "value")
		storage.cache.LRU(ctx).Remove("test")
		So(storage.cache.LRU(ctx).Len(), ShouldResemble, 0)


		found = false
		called = false
		So(storage.cache.LRU(ctx).Len(), ShouldResemble, 0)
		key, err := storage.GetOrCreateIdentity(ctx, "service1", "project1", "")
		So(storage.cache.LRU(ctx).Len(), ShouldResemble, 1)
		So(err, ShouldBeNil)
		So(key, ShouldResemble, "service1-project1")
		So(found, ShouldResemble, false)
		So(called, ShouldResemble, true)

		found = false
		called = false
		key, err = storage.GetOrCreateIdentity(ctx, "service1", "project1", "")
		So(err, ShouldBeNil)
		So(key, ShouldResemble, "service1-project1")
		So(found, ShouldResemble, false)
		So(called, ShouldResemble, false)
		So(storage.cache.LRU(ctx).Len(), ShouldResemble, 1)


		found = false
		called = false
		key, err = storage.GetOrCreateIdentity(ctx, "service2", "project2", "")
		So(err, ShouldBeNil)
		So(key, ShouldResemble, "service2-project2")
		So(found, ShouldResemble, false)
		So(called, ShouldResemble, true)
		So(storage.cache.LRU(ctx).Len(), ShouldResemble, 2)
	})

	Convey("", t, func(){

		ctx := context.Background()

		var found bool = false
		var called bool = false
		var err error = nil

		storage := &testScopedServiceAccountStorage{
			storage: make([][]string, 0),
			onFound: func(service, project, override, key string) {
				found = true
				called = true
			},
			onNotFound: func(service, project, override, key string) {
				found = false
				called = true
			},
		}

		key, err := storage.GetOrCreateIdentity(ctx, "service1", "project1", "")
		So(err, ShouldBeNil)
		So(key, ShouldResemble, "service1-project1")
		So(called, ShouldResemble, true)
		So(found, ShouldResemble, false)

		key, err = storage.GetOrCreateIdentity(ctx, "service1", "project1", "")
		So(err, ShouldBeNil)
		So(key, ShouldResemble, "service1-project1")
		So(called, ShouldResemble, true)
		So(found, ShouldResemble, true)

		key, err = storage.GetOrCreateIdentity(ctx, "service2", "project2", "override")
		So(err, ShouldBeNil)
		So(key, ShouldResemble, "override")
		So(called, ShouldResemble, true)
		So(found, ShouldResemble, false)

		key, err = storage.GetOrCreateIdentity(ctx, "service2", "project2", "")
		So(err, ShouldBeNil)
		So(key, ShouldResemble, "override")
		So(called, ShouldResemble, true)
		So(found, ShouldResemble, true)
	})

}

