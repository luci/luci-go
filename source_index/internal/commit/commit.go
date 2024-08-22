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
	"context"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/spanutil"
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

// ReadByKey retrieves a commit from the database given the commit key.
func ReadByKey(ctx context.Context, k Key) (commits Commit, err error) {
	row, err := span.ReadRow(ctx, "Commits", k.spannerKey(), []string{"PositionRef", "PositionNumber"})
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			return Commit{}, spanutil.ErrNotExists
		}
		return Commit{}, errors.Annotate(err, "reading Commits table row").Err()
	}

	commit := Commit{key: k}

	var positionRef spanner.NullString
	if err := row.Column(0, &positionRef); err != nil {
		return Commit{}, errors.Annotate(err, "reading PositionRef column").Err()
	}

	var positionNum spanner.NullInt64
	if err := row.Column(1, &positionNum); err != nil {
		return Commit{}, errors.Annotate(err, "reading PositionNumber column").Err()
	}

	if positionRef.Valid != positionNum.Valid {
		return Commit{}, errors.New("invariant violated: PositionRef and PositionNumber must be defined/undefined at the same time")
	}
	if positionRef.Valid {
		commit.position = &Position{
			Ref:    positionRef.StringVal,
			Number: positionNum.Int64,
		}
	}

	return commit, nil
}

// ReadByPositionOpts is the option to read by
type ReadByPositionOpts struct {
	// Required. Host is the gitiles host of the repository.
	Host string
	// Required. Repository is the Gitiles project of the commit.
	Repository string
	// Required. Position is the position of the commit along a ref.
	Position Position
}

// ReadByPosition retrieves a commit from the database given the commit
// position.
//
// When there are multiple matches, only the first match (arbitrary order) is
// returned.
// If no commit is found, returns spanutil.ErrNotExists.
func ReadByPosition(ctx context.Context, opts ReadByPositionOpts) (commits Commit, err error) {
	stmt := spanner.NewStatement(`
		SELECT
			CommitHash
		FROM Commits
		WHERE
			Host = @host
				AND Repository = @repository
				AND PositionRef = @positionRef
				AND PositionNumber = @positionNumber
		LIMIT 1
	`)
	stmt.Params = map[string]any{
		"host":           opts.Host,
		"repository":     opts.Repository,
		"positionRef":    opts.Position.Ref,
		"positionNumber": opts.Position.Number,
	}

	var commitHash string
	err = span.Query(ctx, stmt).Do(func(r *spanner.Row) error {
		return r.Column(0, &commitHash)
	})
	if err != nil {
		return Commit{}, errors.Annotate(err, "read commit from spanner by position").Err()
	}
	if commitHash == "" {
		return Commit{}, spanutil.ErrNotExists
	}

	// Don't need to validate the inputs. If they were invalid, there should've
	// been no match.
	return Commit{
		key: Key{
			host:       opts.Host,
			repository: opts.Repository,
			commitHash: commitHash,
		},
		position: &opts.Position,
	}, nil
}
