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
	"testing"
)

func TestGetDB(t *testing.T) {
	t.Parallel()

	var zeroDB sql.DB
	ctx := context.Background()
	ctx = UseDB(ctx, &zeroDB)
	if MustGetDB(ctx) != &zeroDB {
		t.Error("got the wrong db")
	}
}
