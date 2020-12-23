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

package run

import (
	"context"
	"fmt"

	"go.chromium.org/luci/gae/service/datastore"
)

// NewQueryWithLUCIProject returns a query for Run entities that belongs
// to the given LUCI Project.
func NewQueryWithLUCIProject(ctx context.Context, project string) *datastore.Query {
	// ID starts with the LUCI Project name followed by '/' and a 13-digit
	// number. So it must be lexicographically greater than "project/0" and
	// less than "project/:" (int(':') == int('9') + 1).
	return datastore.NewQuery(RunKind).
		Gt("__key__", datastore.MakeKey(ctx, RunKind, fmt.Sprintf("%s/0", project))).
		Lt("__key__", datastore.MakeKey(ctx, RunKind, fmt.Sprintf("%s/:", project)))
}
