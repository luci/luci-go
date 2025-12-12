// Copyright 2015 The LUCI Authors.
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

package authtest

import (
	"context"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
)

// Use installs the fake db into the context.
//
// Note that if you use auth.WithState(ctx, &authtest.FakeState{...}), you don't
// need this method. Modify FakeDB in the FakeState instead. See its doc for
// some examples.
func (db *FakeDB) Use(ctx context.Context) context.Context {
	return auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
		cfg.DBProvider = func(context.Context) (authdb.DB, error) {
			return db, nil
		}
		return cfg
	})
}

// AsProvider returns the current database as an auth.DBProvider.
//
// Here is how you use it:
//
//	testServer, err := servertest.RunServer(ctx, &servertest.Settings{
//		Options: &server.Options{
//			AuthDBProvider: (&authtest.FakeDB{}).AsProvider(),
//		},
//	})
func (db *FakeDB) AsProvider() auth.DBProvider {
	return func(ctx context.Context) (authdb.DB, error) {
		return db, nil
	}
}
