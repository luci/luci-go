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
	"testing"
	"time"

	"go.chromium.org/luci/appengine/gaetesting"

	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

var lruhandle caching.LRUHandle

func init() {
	lruhandle = caching.RegisterLRUCache(512)
}

type testScopedServiceAccountStorage struct {
	storage map[string]*ScopedIdentity
}

func (s *testScopedServiceAccountStorage) Get(c context.Context, service, project string) (*ScopedIdentity, error) {
	key := calculateKey(service, project)
	identity, found := s.storage[key]
	if !found {
		return nil, &Error{Reason: ErrorNotFound}
	}
	return identity, nil
}

func (s *testScopedServiceAccountStorage) GetOrCreate(c context.Context, service, project string) (*ScopedIdentity, error) {
	key := calculateKey(service, project)
	identity, found := s.storage[key]
	if !found {
		identity = NewScopedIdentity(service, project)
		s.storage[key] = identity
	}
	return identity, nil
}

func TestScopedServiceAccountStorage(t *testing.T) {

	Convey("Test cached account storage", t, func() {

		ctx := caching.WithEmptyProcessCache(context.Background())

		mockStorage := &testScopedServiceAccountStorage{
			storage: make(map[string]*ScopedIdentity),
		}

		storage := &cachedIdentityManagerProxy{
			cache:   lruhandle,
			storage: mockStorage,
		}

		storage.cache.LRU(ctx).Put(ctx, "test", "value", time.Minute*5)
		val, found := storage.cache.LRU(ctx).Get(ctx, "test")
		So(found, ShouldResemble, true)
		So(val, ShouldResemble, "value")
		storage.cache.LRU(ctx).Remove("test")
		So(storage.cache.LRU(ctx).Len(), ShouldResemble, 0)

		So(storage.cache.LRU(ctx).Len(), ShouldResemble, 0)
		identity, err := storage.GetOrCreate(ctx, "service1", "project1")
		So(storage.cache.LRU(ctx).Len(), ShouldResemble, 1)
		So(err, ShouldBeNil)
		So(identity.AccountId, ShouldResemble, "ps-600a8066dfcb40a0ee2d9036563")

		identity, err = storage.GetOrCreate(ctx, "service1", "project1")
		So(err, ShouldBeNil)
		So(identity.AccountId, ShouldResemble, "ps-600a8066dfcb40a0ee2d9036563")
		So(storage.cache.LRU(ctx).Len(), ShouldResemble, 1)

		identity, err = storage.GetOrCreate(ctx, "service2", "project2")
		So(err, ShouldBeNil)
		So(identity.AccountId, ShouldResemble, "ps-30b131968fa6e7134042d4f3bd3")
		So(storage.cache.LRU(ctx).Len(), ShouldResemble, 2)
	})

	Convey("Test cache storage state handling", t, func() {

		ctx := context.Background()

		var err error = nil

		storage := &testScopedServiceAccountStorage{
			storage: make(map[string]*ScopedIdentity),
		}

		identity, err := storage.GetOrCreate(ctx, "service1", "project1")
		So(err, ShouldBeNil)
		So(identity.AccountId, ShouldResemble, "ps-600a8066dfcb40a0ee2d9036563")

		identity, err = storage.GetOrCreate(ctx, "service1", "project1")
		So(err, ShouldBeNil)
		So(identity.AccountId, ShouldResemble, "ps-600a8066dfcb40a0ee2d9036563")

		identity, err = storage.GetOrCreate(ctx, "service2", "project2")
		So(err, ShouldBeNil)
		So(identity.AccountId, ShouldResemble, "ps-30b131968fa6e7134042d4f3bd3")

		identity, err = storage.GetOrCreate(ctx, "service2", "project2")
		So(err, ShouldBeNil)
		So(identity.AccountId, ShouldResemble, "ps-30b131968fa6e7134042d4f3bd3")
	})

	Convey("Test persistent storage state handling", t, func() {
		ctx := gaetesting.TestingContext()
		storage := &persistentIdentityManager{}

		identity, err := storage.GetOrCreate(ctx, "service1", "project1")
		So(err, ShouldBeNil)
		So(identity.AccountId, ShouldResemble, "ps-600a8066dfcb40a0ee2d9036563")

		identity, err = storage.GetOrCreate(ctx, "service1", "project1")
		So(err, ShouldBeNil)
		So(identity.AccountId, ShouldResemble, "ps-600a8066dfcb40a0ee2d9036563")

		identity, err = storage.GetOrCreate(ctx, "service2", "project2")
		So(err, ShouldBeNil)
		So(identity.AccountId, ShouldResemble, "ps-30b131968fa6e7134042d4f3bd3")

		identity, err = storage.GetOrCreate(ctx, "service2", "project2")
		So(err, ShouldBeNil)
		So(identity.AccountId, ShouldResemble, "ps-30b131968fa6e7134042d4f3bd3")
	})

}
