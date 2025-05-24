// Copyright 2019 The LUCI Authors.
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

package admin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/dsmapper"

	api "go.chromium.org/luci/cipd/api/admin/v1"
	cipdapi "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/model"
)

func init() {
	initMapper(mapperDef{
		Kind: api.MapperKind_EXPORT_TAGS_TO_BQ,
		Func: exportTagsToBQ,
		Config: dsmapper.JobConfig{
			Query:         dsmapper.Query{Kind: "InstanceTag"},
			ShardCount:    256,
			PageSize:      256, // note: 500 is a strict limit imposed by GetMulti
			TrackProgress: true,
		},
	})
}

func exportTagsToBQ(ctx context.Context, job dsmapper.JobID, _ *api.JobConfig, keys []*datastore.Key) error {
	rows := make([]bigquery.ValueSaver, 0, len(keys))
	err := multiGetTags(ctx, keys, func(key *datastore.Key, tag *model.Tag) error {
		// These checks should never be hit, but just in case...
		switch {
		case key.Parent() == nil:
			logging.Errorf(ctx, "Skipping orphaned tag: %s", key.Encode())
			return nil
		case tag.Instance.Parent() == nil:
			logging.Errorf(ctx, "Skipping tag in orphaned instances: %s", key.Encode())
			return nil
		}
		msg := tag.Proto()
		rows = append(rows, &bq.Row{
			InsertID: bqInsertID("export_tags", job, key),
			Message: &cipdapi.ExportedTag{
				Id:         key.StringID(),
				Instance:   key.Parent().StringID(),
				Package:    key.Parent().Parent().StringID(),
				Key:        msg.Key,
				Value:      msg.Value,
				AttachedBy: msg.AttachedBy,
				AttachedTs: msg.AttachedTs,
			},
		})
		return nil
	})
	if err != nil {
		return transient.Tag.Apply(err)
	}
	return uploadToBQ(ctx, job, rows)
}

// bqInsertID returns a hash of its inputs.
func bqInsertID(seed string, job dsmapper.JobID, key *datastore.Key) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%d\n%s", seed, job, key.Encode())
	return hex.EncodeToString(h.Sum(nil))
}

// uploadToBQ makes insertAll RPC to push rows to BigQuery.
func uploadToBQ(ctx context.Context, job dsmapper.JobID, rows []bigquery.ValueSaver) error {
	if len(rows) == 0 {
		return nil
	}
	logging.Infof(ctx, "Uploading %d rows to exported_tags_%d", len(rows), job)

	// Note: auth.GetTokenSource doesn't work with BQ on GAE, since it starts to
	// demand "real" Appengine context. http.Client is OK though.
	t, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(bigquery.Scope))
	if err != nil {
		return errors.Annotate(err, "failed to grab RPC transport").Err()
	}

	appID := info.TrimmedAppID(ctx)
	client, err := bigquery.NewClient(ctx, appID, option.WithHTTPClient(&http.Client{
		Transport: t,
	}))
	if err != nil {
		return errors.Annotate(err, "failed to grab bigquery.Client").Err()
	}
	defer client.Close()

	// Each mapper job will get its own table.
	ins := client.Dataset("cipd").Table("exported_tags").Inserter()
	ins.TableTemplateSuffix = fmt.Sprintf("_%d", job)
	err = ins.Put(ctx, rows)

	// PutMultiError happens when rows are malformed, a retry won't help.
	if _, ok := err.(bigquery.PutMultiError); ok {
		return errors.Fmt("fatal error when uploading to BQ: %w", err)
	}
	return transient.Tag.Apply(errors.Fmt("transient error when uploading to BQ: %w", err))
}
