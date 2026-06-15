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

package notify

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/spanner"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	buildbucketgrpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"
)

// CleanupStaleBuilders queries Buildbucket to verify if builders tracked in
// BuilderStatuses still exist, and deletes them if they have been deleted.
func CleanupStaleBuilders(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	client, err := buildersClientCreator(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to create builders client")
	}

	projects, err := getUniqueProjects(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to get unique projects")
	}

	var keysToDelete []string
	for _, project := range projects {
		logging.Infof(ctx, "Cleaning up stale builders for project %s", project)
		activeBuilders, err := getActiveBuilders(ctx, client, project)
		if err != nil {
			return errors.Annotate(err, "failed to get active builders for project %s", project)
		}

		storedKeys, err := getStoredBuildersForProject(ctx, project)
		if err != nil {
			return errors.Annotate(err, "failed to get stored builders for project %s", project)
		}

		for _, key := range storedKeys {
			if !activeBuilders.Has(key) {
				logging.Infof(ctx, "Found stale builder to delete: %s", key)
				keysToDelete = append(keysToDelete, key)
			}
		}
	}

	if len(keysToDelete) == 0 {
		logging.Infof(ctx, "No stale builders found to delete")
		return nil
	}

	logging.Infof(ctx, "Deleting %d stale builders", len(keysToDelete))
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		for _, key := range keysToDelete {
			span.BufferWrite(ctx, spanner.Delete("BuilderStatuses", spanner.Key{key}))
		}
		return nil
	})
	if err != nil {
		return errors.Annotate(err, "failed to delete stale builders")
	}

	return nil
}

func getUniqueProjects(ctx context.Context) ([]string, error) {
	st := spanner.NewStatement("SELECT DISTINCT Project FROM BuilderStatuses")
	var projects []string
	err := span.Query(span.Single(ctx), st).Do(func(row *spanner.Row) error {
		var project string
		if err := row.Column(0, &project); err != nil {
			return err
		}
		projects = append(projects, project)
		return nil
	})
	return projects, err
}

var buildersClientCreator = func(ctx context.Context) (buildbucketgrpcpb.BuildersClient, error) {
	t, err := auth.GetRPCTransport(ctx, auth.AsSelf)
	if err != nil {
		return nil, err
	}
	return buildbucketgrpcpb.NewBuildersClient(&prpc.Client{
		C:       &http.Client{Transport: t},
		Host:    BuildBucketHost,
		Options: prpc.DefaultOptions(),
	}), nil
}

func getActiveBuilders(ctx context.Context, client buildbucketgrpcpb.BuildersClient, project string) (stringset.Set, error) {
	activeBuilders := stringset.New(0)
	pageToken := ""
	for {
		req := &buildbucketpb.ListBuildersRequest{
			Project:   project,
			PageSize:  1000,
			PageToken: pageToken,
		}
		res, err := client.ListBuilders(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, b := range res.Builders {
			builderKey := fmt.Sprintf("%s/%s/%s", b.Id.Project, b.Id.Bucket, b.Id.Builder)
			activeBuilders.Add(builderKey)
		}
		pageToken = res.NextPageToken
		if pageToken == "" {
			break
		}
	}
	return activeBuilders, nil
}

func getStoredBuildersForProject(ctx context.Context, project string) ([]string, error) {
	st := spanner.NewStatement("SELECT BuilderKey FROM BuilderStatuses WHERE Project = @project")
	st.Params["project"] = project
	var keys []string
	err := span.Query(span.Single(ctx), st).Do(func(row *spanner.Row) error {
		var key string
		if err := row.Column(0, &key); err != nil {
			return err
		}
		keys = append(keys, key)
		return nil
	})
	return keys, err
}
