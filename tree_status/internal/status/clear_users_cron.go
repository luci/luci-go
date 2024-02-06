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

package status

import (
	"context"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"
)

// ClearStatusUsers clears the column CreateUser in table Status.
//
// The retention policy is 63 days, but we delete it at 30 days
// to allow time to fix issues with the deletion process, as
// well as allow time for data to be wiped from the underlying
// storage media.
func ClearStatusUsers(ctx context.Context) error {
	affectedRows, err := clearCreateUserColumn(ctx)
	if err != nil {
		logging.Errorf(ctx, "Failed to clear creation users: %w", err)
		return err
	}
	logging.Infof(ctx, "Cleared %d create users", affectedRows)

	return nil
}

// clearCreateUserColumn clears column CreateUser in the Status table which
// have passed 30 days to keep inline with our retention policy.
//
// This function returns the number of affected rows.
//
// Column `CreateTime` is used to determine which rows to clear
// the `CreateUser` column for.
func clearCreateUserColumn(ctx context.Context) (int64, error) {
	stmt := spanner.NewStatement(`
	UPDATE Status
	SET  CreateUser = ''
	WHERE CreateTime <= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
	AND CreateUser <> ''
	`)
	var rows int64
	var err error
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		rows, err = span.Update(ctx, stmt)
		return err
	})
	if err != nil {
		return 0, err
	}
	return rows, nil
}
