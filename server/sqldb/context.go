// Copyright 2025 The LUCI Authors.
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

package sqldb

import (
	"context"
	"database/sql"

	"go.chromium.org/luci/common/errors"
)

var contextKey = "sqldb connection"

// UseDB puts a SQL database connection into the current context, giving us back a new context.
func UseDB(ctx context.Context, db *sql.DB) context.Context {
	return context.WithValue(ctx, &contextKey, db)
}

// GetDB retrieves the current database connection from the context.
func GetDB(ctx context.Context) (*sql.DB, error) {
	item := ctx.Value(&contextKey)
	conn, ok := item.(*sql.DB)
	if ok {
		return conn, nil
	}
	return nil, errors.New("no *sql.DB found in context! Use sqldb.UseDB(ctx, ...) to place one in the context.")
}

// MustGetDB gets a sql.DB from the context and panics if it isn't available.
func MustGetDB(ctx context.Context) *sql.DB {
	db, err := GetDB(ctx)
	if err != nil {
		panic(err)
	}
	return db
}
