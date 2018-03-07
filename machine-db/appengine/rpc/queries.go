// Copyright 2017 The LUCI Authors.
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

package rpc

import (
	"github.com/Masterminds/squirrel"

	"go.chromium.org/luci/machine-db/api/common/v1"
)

// selectInInt64 returns the given SELECT modified with a WHERE IN clause.
// expr is the left-hand side of the IN operator, values is the right-hand side. No-op if values is empty.
func selectInInt64(b squirrel.SelectBuilder, expr string, values []int64) squirrel.SelectBuilder {
	if len(values) == 0 {
		return b
	}
	args := make([]interface{}, len(values))
	for i, val := range values {
		args[i] = val
	}
	return b.Where(expr+" IN ("+squirrel.Placeholders(len(args))+")", args...)
}

// selectInState returns the given SELECT modified with a WHERE IN clause.
// expr is the left-hand side of the IN operator, values is the right-hand side. No-op if values is empty.
func selectInState(b squirrel.SelectBuilder, expr string, values []common.State) squirrel.SelectBuilder {
	if len(values) == 0 {
		return b
	}
	args := make([]interface{}, len(values))
	for i, val := range values {
		args[i] = val
	}
	return b.Where(expr+" IN ("+squirrel.Placeholders(len(args))+")", args...)
}

// selectInString returns the given SELECT modified with a WHERE IN clause.
// expr is the left-hand side of the IN operator, values is the right-hand side. No-op if values is empty.
func selectInString(b squirrel.SelectBuilder, expr string, values []string) squirrel.SelectBuilder {
	if len(values) == 0 {
		return b
	}
	args := make([]interface{}, len(values))
	for i, val := range values {
		args[i] = val
	}
	return b.Where(expr+" IN ("+squirrel.Placeholders(len(args))+")", args...)
}

// selectInUint64 returns the given SELECT modified with a WHERE IN clause.
// expr is the left-hand side of the IN operator, values is the right-hand side. No-op if values is empty.
func selectInUint64(b squirrel.SelectBuilder, expr string, values []uint64) squirrel.SelectBuilder {
	if len(values) == 0 {
		return b
	}
	args := make([]interface{}, len(values))
	for i, val := range values {
		args[i] = val
	}
	return b.Where(expr+" IN ("+squirrel.Placeholders(len(args))+")", args...)
}
