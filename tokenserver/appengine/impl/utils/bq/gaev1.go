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

package bq

import (
	"context"

	"google.golang.org/appengine"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/info"
)

// InsertFromGAEv1 exists temporary during GAEv1 => GAEv2 migration.
//
// It uses v2 code, but inefficiently constructing Inserter each time it is
// called. On GAEv2 Inserter will be reused between requests.
func InsertFromGAEv1(ctx context.Context, datasetID, tableID string, row proto.Message) error {
	client, err := NewClient(ctx, info.TrimmedAppID(ctx))
	if err != nil {
		return errors.Annotate(err, "failed to construct BigQuery client").Err()
	}
	inserter := Inserter{
		Table:  client.Dataset(datasetID).Table(tableID),
		DryRun: appengine.IsDevAppServer(),
	}
	return inserter.Insert(ctx, row)
}
