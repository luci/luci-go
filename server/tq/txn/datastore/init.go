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

// Datastore contains Transactional Enqueue support for Cloud Datastore.
//
// Importing this package adds Cloud Datastore transactions support to
// server/tq's AddTask. Depends on "go.chromium.org/gae/filter/txndefer" filter
// installed. It is installed by default in LUCI server contexts.
//
// This package is normally imported unnamed:
//
//   import _ "go.chromium.org/luci/server/tq/txn/datastore"
package datastore

import (
	"context"

	ds "go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/ttq/internals/databases"
	"go.chromium.org/luci/ttq/internals/databases/datastore"

	// Register "datastore" lessor.
	_ "go.chromium.org/luci/ttq/internals/lessors/datastore"
)

var db datastore.DB

func init() {
	databases.Register(databases.Impl{
		Kind: db.Kind(),
		ProbeForTxn: func(ctx context.Context) databases.Database {
			if ds.CurrentTransaction(ctx) != nil {
				return db
			}
			return nil
		},
		NonTxn: func(ctx context.Context) databases.Database {
			return db
		},
	})
}
