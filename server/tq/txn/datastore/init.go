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
// server/tq's AddTask. Depends on "go.chromium.org/luci/gae/filter/txndefer" filter
// installed. It is installed by default in LUCI server contexts.
//
// This package is normally imported unnamed:
//
//	import _ "go.chromium.org/luci/server/tq/txn/datastore"
//
// Will take ownership of entities with kinds "tq.*" (e.g. "tq.Reminder").
package datastore

import (
	"context"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/lessor"
)

var impl dsDB

func init() {
	db.Register(db.Impl{
		Kind:   impl.Kind(),
		Module: gaeemulation.ModuleName,
		ProbeForTxn: func(ctx context.Context) db.DB {
			if datastore.CurrentTransaction(ctx) != nil {
				return impl
			}
			return nil
		},
		NonTxn: func(ctx context.Context) db.DB {
			return impl
		},
	})
}

func init() {
	lessor.Register("datastore", func(context.Context) (lessor.Lessor, error) {
		return &dsLessor{}, nil
	})
}
