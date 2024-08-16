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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/testutil"
)

var readAllSQL = `
	SELECT Host, Repository, CommitHash, PositionRef, PositionNumber
  FROM Commits
  ORDER BY Host, Repository, CommitHash
`

// ReadAllForTesting reads all commits for testing, e.g. to assert all expected
// commits were inserted.
//
// Must be called in a spanner transactional context.
// Do not use in production, will not scale.
func ReadAllForTesting(ctx context.Context) ([]Commit, error) {
	stmt := spanner.NewStatement(readAllSQL)

	var results []Commit
	it := span.Query(ctx, stmt)

	f := func(row *spanner.Row) error {
		commit := Commit{}
		var positionRef spanner.NullString
		var position spanner.NullInt64
		err := row.Columns(
			&commit.Host,
			&commit.Repository,
			&commit.CommitHash,
			&positionRef,
			&position,
		)
		if err != nil {
			return err
		}

		if positionRef.Valid != position.Valid {
			return errors.New("invariant violated: PositionRef and Position must be defined/undefined at the same time")
		}
		if positionRef.Valid {
			commit.Position = &Position{
				Ref:    positionRef.StringVal,
				Number: position.Int64,
			}
		}

		results = append(results, commit)
		return nil
	}
	err := it.Do(f)
	if err != nil {
		return nil, err
	}
	return results, nil
}

// SetForTesting replaces the set of stored commits to match the given set.
func SetForTesting(ctx context.Context, cs ...Commit) error {
	testutil.MustApply(ctx,
		spanner.Delete("Commits", spanner.AllKeys()))
	// Insert some commits.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, c := range cs {
			span.BufferWrite(ctx, c.SaveUnverified())
		}
		return nil
	})
	return err
}
