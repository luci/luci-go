// Copyright 2021 The LUCI Authors.
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

package spanner

import (
	"context"
	"testing"

	"cloud.google.com/go/spanner"
	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/spantest"
	"go.chromium.org/luci/common/testing/registry"
)

func TestMain(m *testing.M) {
	registry.RegisterCmpOption(cmp.AllowUnexported(lease{}))
	spantest.SpannerTestMain(m, "init_db.sql")
}

// cleanupDatabase deletes all data from all tables.
func cleanupDatabase(ctx context.Context, client *spanner.Client) error {
	_, err := client.Apply(ctx, []*spanner.Mutation{
		spanner.Delete("TQReminders", spanner.AllKeys()),
		spanner.Delete("TQLeases", spanner.AllKeys()),
	})
	return err
}
