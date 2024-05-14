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

// Package checkpoints contains methods for maintaining and using
// process checkpoints to ensure processes run once only.
package checkpoints

import (
	"context"
	"regexp"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/pbutil"
)

var processIDRe = regexp.MustCompile(`^[a-z0-9\-]{1,64}$`)

// Key is the primary key of a checkpoint.
type Key struct {
	// Project is the name of the LUCI Project.
	Project string
	// Identifier of the resource to which the checkpoint relates.
	// For example, the ResultDB invocation being ingested.
	// Free-form field, but must be non-empty.
	ResourceID string
	// ProcessID is the identifier of the process requiring checkpointing.
	// Valid pattern: ^[a-z0-9\-]{1,64}$.
	ProcessID string
	// Unique identifier of the checkpoint within the process and resource.
	// Free-form field.
	// If the process has only one checkpoint, this may be empty ("").
	Uniquifier string
}

// Checkpoint records that a point in a process was reached.
type Checkpoint struct {
	Key
	// The time the checkpoint was created.
	CreationTime time.Time
	// The time the checkpoint will be eligible for deletion.
	ExpiryTime time.Time
}

// Exists returns whether a checkpoint with the given key eixsts.
func Exists(ctx context.Context, key Key) (bool, error) {
	_, err := span.ReadRow(ctx, "Checkpoints", spanner.Key{key.Project, key.ResourceID, key.ProcessID, key.Uniquifier}, []string{"CreationTime"})
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			return false, nil
		}
		return false, errors.Annotate(err, "read checkpoint row").Err()
	}
	return true, nil
}

// Insert inserts a checkpoint with the given key and TTL.
func Insert(ctx context.Context, key Key, ttl time.Duration) *spanner.Mutation {
	if err := pbutil.ValidateProject(key.Project); err != nil {
		panic(errors.Annotate(err, "invalid project name").Err())
	}
	if key.ResourceID == "" {
		panic(errors.Reason("empty resource ID").Err())
	}
	if !processIDRe.MatchString(key.ProcessID) {
		panic(errors.Reason("invalid process ID %q, expected pattern %s", key.ProcessID, processIDRe).Err())
	}

	values := map[string]any{
		"Project":      key.Project,
		"ResourceId":   key.ResourceID,
		"ProcessId":    key.ProcessID,
		"Uniquifier":   key.Uniquifier,
		"CreationTime": spanner.CommitTimestamp,
		"ExpiryTime":   clock.Now(ctx).Add(ttl),
	}
	return spanner.InsertMap("Checkpoints", values)
}

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
