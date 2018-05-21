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

package model

import (
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
)

// Package represents a package as it is stored in the datastore.
//
// It is mostly a marker that the package exists plus some minimal metadata
// about this specific package. Metadata for the package prefix is stored
// separately elsewhere (see 'metadata' package). Package instances, tags and
// refs are stored as child entities (see below).
//
// Root entity. ID is the package name.
//
// Compatible with the python version of the backend.
type Package struct {
	_kind  string                `gae:"$kind,Package"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	Name string `gae:"$id"` // e.g. "a/b/c"

	RegisteredBy string    `gae:"registered_by"` // who registered it
	RegisteredTs time.Time `gae:"registered_ts"` // when it was registered

	Hidden bool `gae:"hidden"` // if true, hide from the listings
}

// PackageKey returns a datastore key of some package, given its name.
func PackageKey(c context.Context, pkg string) *datastore.Key {
	return datastore.NewKey(c, "Package", pkg, 0, nil)
}
