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
	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/source_index/internal/testutil"
)

var readAllSQL = `
	SELECT Host, Repository, CommitHash, PositionRef, PositionNumber
  FROM Commits
  ORDER BY Host, Repository, CommitHash
`

// MustReadAllForTesting reads all commits for testing, e.g. to assert all
// expected commits were inserted.
//
// Must be called in a spanner transactional context.
// Do not use in production, will not scale.
func MustReadAllForTesting(ctx context.Context) map[Key]Commit {
	stmt := spanner.NewStatement(readAllSQL)

	results := make(map[Key]Commit)
	it := span.Query(ctx, stmt)

	f := func(row *spanner.Row) error {
		commit := Commit{}
		var positionRef spanner.NullString
		var positionNum spanner.NullInt64
		err := row.Columns(
			&commit.key.host,
			&commit.key.repository,
			&commit.key.commitHash,
			&positionRef,
			&positionNum,
		)
		if err != nil {
			return err
		}

		// Do not validate that both PositionRef and PositionNumber are always
		// specified at the same time when reading for testing purpose.
		// It's up to the test cases to assert the content.
		if positionRef.Valid || positionNum.Valid {
			commit.position = &Position{
				Ref:    positionRef.StringVal,
				Number: positionNum.Int64,
			}
		}

		results[commit.key] = commit
		return nil
	}
	err := it.Do(f)
	if err != nil {
		panic(err)
	}
	return results
}

// MustSetForTesting replaces the set of stored commits to match the given set.
func MustSetForTesting(ctx context.Context, cs ...Commit) {
	testutil.MustApply(ctx,
		spanner.Delete("Commits", spanner.AllKeys()))
	// Insert some commits.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, c := range cs {
			span.BufferWrite(ctx, c.Save())
		}
		return nil
	})

	if err != nil {
		panic(err)
	}
}

// ShouldMatchCommit returns a comparison.Func which checks if the expected
// commit matches the actual commit.
func ShouldMatchCommit(commit Commit) comparison.Func[Commit] {
	return should.Match(commit, cmp.AllowUnexported(Commit{}, Key{}))
}

// ShouldMatchCommits returns a comparison.Func which checks if the expected
// commits matches the actual commits. The order of the commits does not matter.
func ShouldMatchCommits(commits []Commit) comparison.Func[map[Key]Commit] {
	commitsMap := make(map[Key]Commit, len(commits))
	for _, c := range commits {
		commitsMap[c.Key()] = c
	}

	return should.Match(commitsMap, cmp.AllowUnexported(Commit{}, Key{}))
}

// ShouldMatchKey returns a comparison.Func which checks if the expected commit
// key matches the actual commit key.
func ShouldMatchKey(key Key) comparison.Func[Key] {
	return should.Match(key, cmp.AllowUnexported(Key{}))
}

// ShouldMatchGitCommit returns a comparison.Func which checks if the expected
// GitCommit matches the actual GitCommit.
func ShouldMatchGitCommit(commit GitCommit) comparison.Func[GitCommit] {
	return should.Match(commit, cmp.AllowUnexported(GitCommit{}, Key{}))
}
