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

package checkpoints

import (
	"context"
	"sort"
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
)

// ReadAllForTesting reads all checkpoints for testing, e.g. to assert
// all expected checkpoints were made.
// Do not use in production, will not scale.
// Must be called in a spanner transactional context.
func ReadAllForTesting(ctx context.Context) ([]Checkpoint, error) {
	sql := `SELECT Project, ResourceId, ProcessId, Uniquifier, CreationTime,
		ExpiryTime
	FROM Checkpoints
	ORDER BY Project, ResourceId, ProcessId, Uniquifier`

	stmt := spanner.NewStatement(sql)

	var results []Checkpoint
	it := span.Query(ctx, stmt)

	f := func(row *spanner.Row) error {
		c := Checkpoint{}
		err := row.Columns(
			&c.Project,
			&c.ResourceID,
			&c.ProcessID,
			&c.Uniquifier,
			&c.CreationTime,
			&c.ExpiryTime,
		)
		if err != nil {
			return err
		}

		results = append(results, c)
		return nil
	}
	err := it.Do(f)
	if err != nil {
		return nil, err
	}
	return results, nil
}

// SetForTesting replaces the set of stored checkpoints to match the given set.
func SetForTesting(ctx context.Context, t testing.TB, cs ...Checkpoint) error {
	t.Helper()

	testutil.MustApply(ctx, t,
		spanner.Delete("Checkpoints", spanner.AllKeys()))
	// Insert some Checkpoint records.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, r := range cs {
			ms := spanutil.InsertMap("Checkpoints", map[string]any{
				"Project":      r.Project,
				"ResourceId":   r.ResourceID,
				"ProcessId":    r.ProcessID,
				"Uniquifier":   r.Uniquifier,
				"CreationTime": r.CreationTime,
				"ExpiryTime":   r.ExpiryTime,
			})
			span.BufferWrite(ctx, ms)
		}
		return nil
	})
	return err
}

// SortKeys sorts keys in place in ascending
// order by Project, ResourceId, ProcessId, Uniquifier.
func SortKeys(cs []Key) {
	sort.Slice(cs, func(i, j int) bool {
		return isKeyLess(cs[i], cs[j])
	})
}

func isKeyLess(a, b Key) bool {
	if a.Project != b.Project {
		return a.Project < b.Project
	}
	if a.ResourceID != b.ResourceID {
		return a.ResourceID < b.ResourceID
	}
	if a.ProcessID != b.ProcessID {
		return a.ProcessID < b.ProcessID
	}
	if a.Uniquifier != b.Uniquifier {
		return a.Uniquifier < b.Uniquifier
	}
	return false
}
