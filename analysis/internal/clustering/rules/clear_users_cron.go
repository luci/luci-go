// Copyright 2022 The LUCI Authors.
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

package rules

import (
	"context"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"
)

// clearLastUpdatedUserColumn clears column LastUpdatedUser
// in the FailureAssociationRules table which have
// passed 30 days to keep inline with our retention policy.
//
// This function returns the number of affected rows.
//
// Column `LastUpdated` is used to determine which rows to clear
// the `LastUpdatedUser` column for.
func clearLastUpdatedUserColumn(ctx context.Context) (int64, error) {
	stmt := spanner.NewStatement(`
	UPDATE  FailureAssociationRules
	SET  LastAuditableUpdateUser = ''
	WHERE LastAuditableUpdate <= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
	AND LastAuditableUpdateUser <> ''
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

// clearCreationUserColumn clears column CreationUser
// in the FailureAssociationRules table which have
// passed 30 days to keep inline with our retention policy.
// As a result, the created user inforation will
// typically be cleared 30 days after the rule has been created.
//
// This function returns the number of affected rows.
//
// Column `CreationTime` is used to determine which rows to clear
// the `CreationUser` column for.
func clearCreationUserColumn(ctx context.Context) (int64, error) {
	stmt := spanner.NewStatement(`
	UPDATE  FailureAssociationRules
	SET  CreationUser = ''
	WHERE CreationTime <= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
	AND CreationUser <> ''
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

// ClearRulesUsers clears the columns CreationUser and LastAuditableUpdateUser
// in table FailureAssociationRule.
//
// The retention policy is 63 days, but we delete it at 30 days
// to allow time to fix issues with the deletion process, as
// well as allow time for data to be wiped from the underlying
// storage media.
func ClearRulesUsers(ctx context.Context) error {
	affectedRows, err := clearCreationUserColumn(ctx)
	if err != nil {
		logging.Errorf(ctx, "Failed to clear creation users: %w", err)
		return err
	}
	logging.Infof(ctx, "Cleared %d creation users", affectedRows)

	affectedRows, err = clearLastUpdatedUserColumn(ctx)
	if err != nil {
		logging.Errorf(ctx, "Failed to clear last update users: %w", err)
		return err
	}
	logging.Infof(ctx, "Cleared %d last update users", affectedRows)

	return nil
}
