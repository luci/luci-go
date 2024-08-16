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

// Key denotes the primary key for the Commits table.
type Key struct {
	// Host is the gitiles host of the repository. Must be a subdomain of
	// `.googlesource.com` (e.g. chromium.googlesource.com).
	Host string
	// Repository is the Gitiles project of the commit (e.g. "chromium/src" part
	// in https://chromium.googlesource.com/chromium/src/+/main).
	Repository string
	// CommitHash is the full hex sha1 of the commit in lowercase.
	CommitHash string
}

// spannerKey returns the spanner key for the commit key.
func (k Key) spannerKey() spanner.Key {
	return spanner.Key{k.Host, k.Repository, k.CommitHash}
}

// CommitReadCols is the set of columns read from in a commit read.
var CommitReadCols = []string{
	"PositionRef", "PositionNumber",
}

// ReadCommit retrieves a commit from the database given the commit key.
func ReadCommit(ctx context.Context, k Key) (commits *Commit, err error) {
	row, err := span.ReadRow(ctx, "Commits", k.spannerKey(), CommitReadCols)
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			return nil, spanutil.ErrNotExists
		}
		return nil, errors.Annotate(err, "reading Commits table row").Err()
	}

	commit := &Commit{Key: k}

	var positionRef spanner.NullString
	if err := row.Column(0, &positionRef); err != nil {
		return nil, errors.Annotate(err, "reading PositionRef column").Err()
	}

	var position spanner.NullInt64
	if err := row.Column(1, &position); err != nil {
		return nil, errors.Annotate(err, "reading Position column").Err()
	}

	if positionRef.Valid != position.Valid {
		return nil, errors.New("invariant violated: PositionRef and Position must be defined/undefined at the same time")
	}
	if positionRef.Valid {
		commit.Position = &Position{
			Ref:    positionRef.StringVal,
			Number: position.Int64,
		}
	}

	return commit, nil
}

// Exists checks whether the commit key exists in the database.
func Exists(ctx context.Context, k Key) (bool, error) {
	_, err := span.ReadRow(ctx, "Commits", k.spannerKey(), []string{"CommitHash"})
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			return false, nil
		}
		return false, errors.Annotate(err, "reading Commits table row").Err()
	}

	return true, nil
}
