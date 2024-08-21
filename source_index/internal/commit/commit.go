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
	// key is the primary key of the commit in the Commits table.
	key Key
	// position is the position of the commit along a ref.
	position *Position
}

// Position represents the position of a commit along a ref.
type Position struct {
	// Ref is the ref where the commit position is based on.
	Ref string
	// Number is the commit position number.
	Number int64
}

// Key returns the primary key of the commit in the Commits table.
func (c Commit) Key() Key {
	return c.key
}

// Position returns the position of the commit along a ref.
func (c Commit) Position() *Position {
	return c.position
}

// CommitSaveCols is the set of columns written to in a commit save.
// Allocated here once to avoid reallocating on every commit save.
var CommitSaveCols = []string{
	"Host", "Repository", "CommitHash", "PositionRef", "PositionNumber", "LastUpdatedTime",
}

// Save creates a mutation to save/overwrite the commit into the Commits table.
// The commit should've already been verified during struct creation.
func (c Commit) Save() *spanner.Mutation {
	var positionRef spanner.NullString
	var position spanner.NullInt64
	if c.position != nil {
		positionRef.Valid = true
		positionRef.StringVal = c.position.Ref
		position.Valid = true
		position.Int64 = c.position.Number
	}

	vals := []any{
		c.key.host,
		c.key.repository,
		c.key.commitHash,
		positionRef,
		position,
		spanner.CommitTimestamp,
	}

	return spanner.InsertOrUpdate("Commits", CommitSaveCols, vals)
}

// NewFromGitCommit creates a new Commit from a GitCommit.
//
// N.B. if the GitCommit does not have a valid git commit position, the returned
// Commit will not have a commit position either.
func NewFromGitCommit(gitCommit GitCommit) Commit {
	position, err := gitCommit.Position()
	if err != nil {
		position = nil
	}

	return Commit{
		key:      gitCommit.key,
		position: position,
	}
}
