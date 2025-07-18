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
	"net/url"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMakeDBURI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		driver   string
		url      string
		password string
		outURL   string
		err      error
	}{
		{
			driver:   "pgx",
			url:      "postgres://username@localhost:5000/dbname",
			password: "hunter2",
			outURL:   "pgx://username:hunter2@localhost:5000/dbname",
			err:      nil,
		},
	}

	for _, tt := range cases {
		t.Run(tt.url, func(t *testing.T) {
			t.Parallel()

			theURL, err := url.Parse(tt.url)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			actualURL, err := makeDBURI(tt.driver, theURL, tt.password)
			assert.That(t, err, should.ErrLikeError(tt.err))
			assert.That(t, actualURL, should.Match(tt.outURL))
		})
	}
}

func TestWithMagicSqliteDriver(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	assert.NoErr(t, err)

	ctx := context.Background()
	ctx = UseDB(ctx, db)

	gotDB := MustGetDB(ctx)
	assert.Loosely(t, gotDB, should.NotBeNil)
}
