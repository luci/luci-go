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
	"time"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/server/caching"
)

func Get() ScopedIdentityManager {
	return scopedIdentityManager
}

var scopedIdentityManager ScopedIdentityManager

func init() {
	scopedIdentityManager = &cachedIdentityManagerProxy{
		cache:   caching.RegisterLRUCache(512),
		storage: &persistentIdentityManager{},
	}
}

type ScopedIdentityManager interface {
	GetOrCreateIdentity(c context.Context, service, project, override string) (string, error)
}

type scopedIdentity struct {
	_kind     string `gae:"$kind,ScopedIdentity"`
	AccountId string `gae:"$id"`
	Service   string
	Project   string
}

type cachedIdentityManagerProxy struct {
	cache   caching.LRUHandle
	storage ScopedIdentityManager
}

type persistentIdentityManager struct {
}

func getKey(service, project, override string) (string, error) {
	if override == "" {
		return GenerateAccountId(service, project)
	}
	return override, nil
}

func (s *cachedIdentityManagerProxy) GetOrCreateIdentity(c context.Context, service, project, override string) (string, error) {
	key, err := getKey(service, project, override)
	if err != nil {
		return "", err
	}

	value, err := s.cache.LRU(c).GetOrCreate(c, key, func() (interface{}, time.Duration, error) {
		value, err := s.storage.GetOrCreateIdentity(c, service, project, override)
		return value, time.Minute * 5, err
	})
	return value.(string), err
}

func (s *persistentIdentityManager) GetOrCreateIdentity(c context.Context, service, project, override string) (string, error) {
	key, err := getKey(service, project, override)
	if err != nil {
		return "", err
	}

	identity := &scopedIdentity{
		AccountId: key,
		Service:   service,
		Project:   project,
	}

	// Transactionally get-or-create new identity entry
	ds.RunInTransaction(c,
		func(c context.Context) error {
			if err = ds.Get(c, &identity); err != nil {
				if err = ds.Put(c, &identity); err != nil {
					return err
				}
			}
			return nil
		},
		&ds.TransactionOptions{XG: false, Attempts: 5, ReadOnly: false})

	// Assertion
	if identity.Service != service || identity.Project != project {
		panic("This should never happen and indicates something is seriously broken")
	}
	return key, err
}
