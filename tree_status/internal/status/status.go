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

// Package status manages status values.
package status

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/tree_status/pbutil"
	pb "go.chromium.org/luci/tree_status/proto/v1"
)

// NotExistsErr is returned when the requested object was not found in the database.
var NotExistsErr error = errors.New("status value was not found")

// Status mirrors the structure of status values in the database.
type Status struct {
	// The name of the tree this status is part of.
	TreeName string
	// The unique identifier for the status. This is a randomly generated
	// 128-bit ID, encoded as 32 lowercase hexadecimal characters.
	StatusID string
	// The general state of the tree.
	GeneralStatus pb.GeneralState
	// The message explaining details about the status.
	Message string
	// The username of the user who added this.  Will be
	// set to 'user' after the username TTL (of 30 days).
	CreateUser string
	// The time the status update was made.
	// If filling this in from commit timestamp, make sure to call .UTC(), i.e.
	// status.CreateTime = timestamp.UTC()
	CreateTime time.Time
	// The name of the LUCI builder that caused the tree to close.
	// Format: projects/{project}/buckets/{bucket}/builders/{builder}.
	ClosingBuilderName string
}

// Validate validates a status value.
// It ignores the CreateUser and CreateTime fields.
// Reported field names are as they appear in the RPC documentation rather than the Go struct.
func Validate(status *Status) error {
	if err := pbutil.ValidateTreeID(status.TreeName); err != nil {
		return errors.Fmt("tree: %w", err)
	}
	if err := pbutil.ValidateStatusID(status.StatusID); err != nil {
		return errors.Fmt("id: %w", err)
	}
	if err := pbutil.ValidateGeneralStatus(status.GeneralStatus); err != nil {
		return errors.Fmt("general_state: %w", err)
	}
	if err := pbutil.ValidateMessage(status.Message); err != nil {
		return errors.Fmt("message: %w", err)
	}
	if status.GeneralStatus == pb.GeneralState_CLOSED {
		if err := pbutil.ValidateClosingBuilderName(status.ClosingBuilderName); err != nil {
			return errors.Fmt("closing_builder_name: %w", err)
		}
	}
	return nil
}

// Create creates a status entry in the Spanner Database.
// Must be called with an active RW transaction in the context.
// CreateUser and CreateTime in the passed in status will be ignored
// in favour of the commit time and the currentUser argument.
func Create(status *Status, currentUser string) (*spanner.Mutation, error) {
	if err := Validate(status); err != nil {
		return nil, err
	}

	row := map[string]any{
		"TreeName":           status.TreeName,
		"StatusId":           status.StatusID,
		"GeneralStatus":      int64(status.GeneralStatus),
		"Message":            status.Message,
		"CreateUser":         currentUser,
		"CreateTime":         spanner.CommitTimestamp,
		"ClosingBuilderName": status.ClosingBuilderName,
	}
	return spanner.InsertOrUpdateMap("Status", row), nil
}

// Read retrieves a status update from the database given the exact time the status update was made.
func Read(ctx context.Context, treeName, statusID string) (*Status, error) {
	row, err := span.ReadRow(ctx, "Status", spanner.Key{treeName, statusID}, []string{"TreeName", "StatusId", "GeneralStatus", "Message", "CreateUser", "CreateTime", "ClosingBuilderName"})
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			return nil, NotExistsErr
		}
		return nil, errors.Fmt("get status: %w", err)
	}
	return fromRow(row)
}

// ReadLatest retrieves the most recent status update for a tree from the database.
func ReadLatest(ctx context.Context, treeName string) (*Status, error) {
	stmt := spanner.NewStatement(`
		SELECT
			TreeName,
			StatusId,
			GeneralStatus,
			Message,
			CreateUser,
			CreateTime,
			ClosingBuilderName
		FROM Status
		WHERE
			TreeName = @treeName
		ORDER BY CreateTime DESC
		LIMIT 1`)
	stmt.Params["treeName"] = treeName
	iter := span.Query(ctx, stmt)
	row, err := iter.Next()
	defer iter.Stop()
	if errors.Is(err, iterator.Done) {
		return nil, NotExistsErr
	} else if err != nil {
		return nil, errors.Fmt("get latest status: %w", err)
	}
	return fromRow(row)
}

// ListOptions allows specifying extra options to the List method.
type ListOptions struct {
	// The offset into the list of statuses in the database to start reading at.
	Offset int64
	// The maximum number of items to return from the database.
	// If left as 0, the value of 100 will be used.
	Limit int64
}

// List retrieves a list of status values from the database.  Status values are listed in reverse
// CreateTime order (i.e. most recently created first).
// The options argument may be nil to use default options.
func List(ctx context.Context, treeName string, options *ListOptions) ([]*Status, bool, error) {
	if options == nil {
		options = &ListOptions{}
	}
	if options.Limit == 0 {
		options.Limit = 100
	}

	stmt := spanner.NewStatement(`
		SELECT
			TreeName,
			StatusId,
			GeneralStatus,
			Message,
			CreateUser,
			CreateTime,
			ClosingBuilderName
		FROM Status
		WHERE
			TreeName = @treeName
		ORDER BY CreateTime DESC
		LIMIT @limit
		OFFSET @offset`)
	stmt.Params["treeName"] = treeName
	// Read one extra result to detect whether there is a next page.
	stmt.Params["limit"] = options.Limit + 1
	stmt.Params["offset"] = options.Offset

	iter := span.Query(ctx, stmt)

	statusList := []*Status{}
	if err := iter.Do(func(row *spanner.Row) error {
		status, err := fromRow(row)
		statusList = append(statusList, status)
		return err
	}); err != nil {
		return nil, false, errors.Fmt("list status: %w", err)
	}
	hasNextPage := false
	if int64(len(statusList)) > options.Limit {
		statusList = statusList[:options.Limit]
		hasNextPage = true
	}
	return statusList, hasNextPage, nil
}

// ListAfter retrieves a list of status values from the Spanner that were
// created after a particular timestamp.
//
//	Status values are listed in ascending order of CreateTime.
func ListAfter(ctx context.Context, afterTime time.Time) ([]*Status, error) {
	stmt := spanner.NewStatement(`
		SELECT
			TreeName,
			StatusId,
			GeneralStatus,
			Message,
			CreateUser,
			CreateTime,
			ClosingBuilderName
		FROM Status
		WHERE
			CreateTime > @afterTime
		ORDER BY CreateTime`)

	stmt.Params["afterTime"] = afterTime

	iter := span.Query(ctx, stmt)
	results := []*Status{}
	err := iter.Do(func(row *spanner.Row) error {
		status, err := fromRow(row)
		if err != nil {
			return errors.Fmt("from row: %w", err)
		}
		results = append(results, status)
		return nil
	})
	if err != nil {
		return nil, errors.Fmt("query status: %w", err)
	}
	return results, nil
}

func fromRow(row *spanner.Row) (*Status, error) {
	status := &Status{}
	if err := row.Column(0, &status.TreeName); err != nil {
		return nil, errors.Fmt("reading TreeName column: %w", err)
	}
	if err := row.Column(1, &status.StatusID); err != nil {
		return nil, errors.Fmt("reading StatusID column: %w", err)
	}
	generalStatus := int64(0)
	if err := row.Column(2, &generalStatus); err != nil {
		return nil, errors.Fmt("reading GeneralStatus column: %w", err)
	}
	status.GeneralStatus = pb.GeneralState(generalStatus)
	if err := row.Column(3, &status.Message); err != nil {
		return nil, errors.Fmt("reading Message column: %w", err)
	}
	if err := row.Column(4, &status.CreateUser); err != nil {
		return nil, errors.Fmt("reading CreateUser column: %w", err)
	}
	if err := row.Column(5, &status.CreateTime); err != nil {
		return nil, errors.Fmt("reading CreateTime column: %w", err)
	}
	if err := row.Column(6, &status.ClosingBuilderName); err != nil {
		return nil, errors.Fmt("reading ClosingBuilderName column: %w", err)
	}
	return status, nil
}

// GenerateID returns a random 128-bit status ID, encoded as
// 32 lowercase hexadecimal characters.
func GenerateID() (string, error) {
	randomBytes := make([]byte, 16)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(randomBytes), nil
}
