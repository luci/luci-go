// Copyright 2026 The LUCI Authors.
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

// Package builders manages builder statuses in the database.
package builders

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/aip160"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"
)

// BuilderStatus mirrors the structure of builder statuses in the database.
type BuilderStatus struct {
	BuilderKey      string
	Project         string
	Bucket          string
	Builder         string
	Status          string
	UpdateTime      time.Time
	BuildId         int64
	OnCallRotations []string
}

var builderStatusesTable = aip160.NewDatabaseTable().WithFields(
	aip160.NewField().WithFieldPath("project").WithBackend(aip160.NewStringColumn("Project")).Filterable().Build(),
	aip160.NewField().WithFieldPath("bucket").WithBackend(aip160.NewStringColumn("Bucket")).Filterable().Build(),
	aip160.NewField().WithFieldPath("builder").WithBackend(aip160.NewStringColumn("Builder")).Filterable().Build(),
	aip160.NewField().WithFieldPath("status").WithBackend(aip160.NewStringColumn("Status")).Filterable().Build(),
	aip160.NewField().WithFieldPath("on_call_rotations").WithBackend(aip160.NewRepeatedStringColumn("OnCallRotations")).Filterable().Build(),
).Build()

// ListOptions contains options for listing builder statuses.
type ListOptions struct {
	PageToken        string
	PageSize         int
	FullRealms       []string
	WildcardProjects []string
	Filter           *aip160.Filter
}

// List returns a list of builder statuses and the next page token.
func List(ctx context.Context, opts ListOptions) ([]*BuilderStatus, string, error) {
	whereClause, aipParams, err := builderStatusesTable.WhereClause(opts.Filter, "", "w_")
	if err != nil {
		return nil, "", appstatus.Attachf(err, codes.InvalidArgument, "generating where clause")
	}

	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	if pageSize > 1000 {
		pageSize = 1000
	}

	st := spanner.NewStatement(`
		SELECT
			BuilderKey, Project, Bucket, Builder, Status, UpdateTime, BuildId, OnCallRotations
		FROM BuilderStatuses
		WHERE BuilderKey > @pageToken
	`)

	// Apply ACL filter
	aclClause := ""
	if len(opts.FullRealms) > 0 {
		aclClause = "Realm IN UNNEST(@allowedRealms)"
	}
	if len(opts.WildcardProjects) > 0 {
		if aclClause != "" {
			aclClause += " OR "
		}
		aclClause += "Project IN UNNEST(@allowedProjects)"
	}
	if aclClause != "" {
		st.SQL += " AND (" + aclClause + ")"
		st.Params["allowedRealms"] = opts.FullRealms
		st.Params["allowedProjects"] = opts.WildcardProjects
	} else {
		// Should not happen if called correctly, as RPC layer should check this.
		return nil, "", errors.New("internal error: allowed realms and wildcard projects are both empty")
	}

	if whereClause != "" {
		st.SQL += " AND (" + whereClause + ")"
	}
	st.SQL += " ORDER BY BuilderKey LIMIT @pageSize"

	st.Params["pageToken"] = opts.PageToken
	st.Params["pageSize"] = pageSize
	for _, p := range aipParams {
		st.Params[p.Name] = p.Value
	}

	var builders []*BuilderStatus
	iter := span.Query(ctx, st)
	err = iter.Do(func(row *spanner.Row) error {
		b := &BuilderStatus{}
		if err := row.Columns(&b.BuilderKey, &b.Project, &b.Bucket, &b.Builder, &b.Status, &b.UpdateTime, &b.BuildId, &b.OnCallRotations); err != nil {
			return err
		}
		builders = append(builders, b)
		return nil
	})
	if err != nil {
		return nil, "", errors.Fmt("query builder statuses: %w", err)
	}

	nextPageToken := ""
	if len(builders) == pageSize {
		nextPageToken = builders[len(builders)-1].BuilderKey
	}

	return builders, nextPageToken, nil
}
