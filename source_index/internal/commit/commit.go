// Copyright 2024 The LUCI Authors.
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

package commit

import (
	"cloud.google.com/go/spanner"
)

// Commit represents a row in the Commits table.
type Commit struct {
	// Key is the primary key of the commit in Commits table.
	Key
	// Position is the position of the commit along a ref.
	Position *Position
}

// Position represents the position of a commit along a ref.
type Position struct {
	// Ref is the ref where the commit position is based on.
	Ref string
	// Number is the commit position number.
	Number int64
}

// CommitSaveCols is the set of columns written to in a commit save.
// Allocated here once to avoid reallocating on every commit save.
var CommitSaveCols = []string{
	"Host", "Repository", "CommitHash", "PositionRef", "PositionNumber", "UpdateTime",
}

// SaveUnverified creates a mutation to save/overwrite the commit into the
// Commits table. The commit is not verified.
func (c *Commit) SaveUnverified() *spanner.Mutation {
	var positionRef spanner.NullString
	var position spanner.NullInt64
	if c.Position != nil {
		positionRef.Valid = true
		positionRef.StringVal = c.Position.Ref
		position.Valid = true
		position.Int64 = c.Position.Number
	}

	vals := []any{
		c.Host,
		c.Repository,
		c.CommitHash,
		positionRef,
		position,
		spanner.CommitTimestamp,
	}

	return spanner.InsertOrUpdate("Commits", CommitSaveCols, vals)
}
