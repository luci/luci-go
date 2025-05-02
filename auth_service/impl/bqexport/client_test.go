// Copyright 2025 The LUCI Authors.
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

package bqexport

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestConstructLatestSnapshotViewQuery(t *testing.T) {
	t.Parallel()

	ftt.Run("latest_groups query constructed correctly", t, func(t *ftt.Test) {
		expected := `
	WITH these_keys AS (
		SELECT DISTINCT exported_at FROM luci_auth_service.groups
	),

	other_keys AS (
		SELECT DISTINCT exported_at FROM luci_auth_service.realms
	),

	latest AS (
		SELECT MAX(these_keys.exported_at) AS key FROM these_keys
		INNER JOIN other_keys
		ON these_keys.exported_at = other_keys.exported_at
	)

	SELECT * FROM luci_auth_service.groups
	WHERE exported_at = COALESCE(
		(SELECT key FROM latest),
		(SELECT MAX(exported_at) FROM luci_auth_service.groups)
	)
	`

		actual, err := constructLatestSnapshotViewQuery(context.Background(), "groups", "realms")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Equal(expected))
	})
}

func TestConstructLatestRolesViewQuery(t *testing.T) {
	t.Parallel()

	ftt.Run("latest_roles query constructed correctly", t, func(t *ftt.Test) {
		expected := `
	SELECT * FROM luci_auth_service.roles
	WHERE exported_at = (SELECT MAX(exported_at) FROM luci_auth_service.roles)
	`

		actual, err := constructLatestRolesViewQuery(context.Background())
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Equal(expected))
	})
}
