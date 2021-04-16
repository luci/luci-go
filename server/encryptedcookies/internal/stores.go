// Copyright 2021 The LUCI Authors.
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

package internal

import (
	"context"
	"fmt"
	"sync"

	"go.chromium.org/luci/server/encryptedcookies/session"
	"go.chromium.org/luci/server/module"
)

var (
	implsM sync.Mutex
	impls  []StoreImpl
)

// StoreImpl represents a factory for producing session.Store.
type StoreImpl struct {
	ID      string
	Factory func(ctx context.Context, namespace string) (session.Store, error)
	Deps    []module.Dependency
}

// RegisterStoreImpl registers an available store implementation.
//
// Called during init() time by packages that implements stores.
func RegisterStoreImpl(impl StoreImpl) {
	implsM.Lock()
	defer implsM.Unlock()
	for _, im := range impls {
		if im.ID == impl.ID {
			panic(fmt.Sprintf("session store implementation %q has already been registered", impl.ID))
		}
	}
	impls = append(impls, impl)
}

// StoreImpls returns registered store implementations.
func StoreImpls() []StoreImpl {
	implsM.Lock()
	defer implsM.Unlock()
	return append([]StoreImpl(nil), impls...)
}
