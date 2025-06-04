// Copyright 2022 The LUCI Authors.
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

package quota

import (
	"sync"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/quota/quotapb"
)

type Application struct {
	id        string
	resources stringset.Set
}

var quotaApplicationsMu sync.RWMutex
var quotaApplications = map[string]*Application{}
var quotaApplicationsClosed bool
var quotaAppTypesOnce sync.Once

type ApplicationOptions struct {
	// ResourceTypes enumerates all the ResourceTypes this application can use.
	//
	// Policies and Accounts both specify a single resource type, and these must
	// match. I.e. you cannot use a policy for `qps` to manage a `storage_bytes`
	// account.
	ResourceTypes []string
}

func Register(appID string, ao *ApplicationOptions) *Application {
	if len(appID) == 0 {
		panic("empty appID")
	}
	resources := stringset.NewFromSlice(ao.ResourceTypes...)
	if len(resources) == 0 {
		panic(errors.New("quota app registration requires at least one resource type"))
	}
	if resources.Has("") {
		panic(errors.New("resource types must not include an empty string"))
	}

	quotaApplicationsMu.Lock()
	defer quotaApplicationsMu.Unlock()

	if quotaApplicationsClosed {
		panic(errors.New("quota app registration already closed"))
	}
	if _, ok := quotaApplications[appID]; ok {
		panic(errors.Fmt("appID %q already registered", appID))
	}
	ret := &Application{appID, resources}
	quotaApplications[appID] = ret
	return ret
}

// AccountID is a convenience method to make an AccountID tied to this
// application.
//
// Will panic if resourceType is not registered for this Application.
func (a *Application) AccountID(realm, namespace, name, resourceType string) *quotapb.AccountID {
	if !a.resources.Has(resourceType) {
		panic(errors.Fmt("application %q does not have resourceType %q", a.id, resourceType))
	}
	return &quotapb.AccountID{
		AppId:        a.id,
		Realm:        realm,
		Namespace:    namespace,
		Name:         name,
		ResourceType: resourceType,
	}
}
