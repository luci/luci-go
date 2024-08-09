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

// Package bqexporter exports status data to BigQuery.
package bqexporter

import (
	"context"
	"regexp"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/tree_status/internal/status"
	bqpb "go.chromium.org/luci/tree_status/proto/bq"
	v1 "go.chromium.org/luci/tree_status/proto/v1"
)

type ExportClient interface {
	EnsureSchema(ctx context.Context) error
	InsertStatusRows(ctx context.Context, rows []*bqpb.StatusRow) error
	ReadMostRecentCreateTime(ctx context.Context) (time.Time, error)
}

// ExportStatus exports status data to BigQuery.
// It queries Spanner and BigQuery and exports rows in Spanner that are not in
// BigQuery.
// This is run from a cron job and only 1 instance may be run at one time. Otherwise,
// we may run into the issue of inserting duplicated rows into BigQuery.
func ExportStatus(ctx context.Context) error {
	client, err := NewClient(ctx, info.AppID(ctx))
	if err != nil {
		return errors.Annotate(err, "new client").Err()
	}
	defer client.Close()

	err = export(ctx, client)
	if err != nil {
		return errors.Annotate(err, "export").Err()
	}
	return nil
}

func export(ctx context.Context, client ExportClient) error {
	// During the very first run, EnsureSchema will make sure the table is created.
	err := client.EnsureSchema(ctx)
	if err != nil {
		return errors.Annotate(err, "ensure schema").Err()
	}

	// Read the most recent row from BigQuery.
	lastCreateTime, err := client.ReadMostRecentCreateTime(ctx)
	if err != nil {
		return errors.Annotate(err, "read most recent create time").Err()
	}
	logging.Infof(ctx, "The most recent create time is %v", lastCreateTime)

	// Query spanner for the rows which are more recent than lastCreateTime.
	// We need to export those rows to BigQuery.
	statuses, err := status.ListAfter(span.Single(ctx), lastCreateTime)
	if err != nil {
		return errors.Annotate(err, "list after").Err()
	}

	// Export to Bigquery
	logging.Infof(ctx, "There are %d rows that needs to be exporter to BigQuery", len(statuses))
	rows := []*bqpb.StatusRow{}
	for _, status := range statuses {
		row, err := toRow(status)
		if err != nil {
			return errors.Annotate(err, "to row").Err()
		}
		rows = append(rows, row)
	}
	err = client.InsertStatusRows(ctx, rows)
	if err != nil {
		return errors.Annotate(err, "insert status row").Err()
	}
	return nil
}

func toRow(s *status.Status) (*bqpb.StatusRow, error) {
	closingBuilder, err := toClosingBuilder(s.ClosingBuilderName)
	if err != nil {
		return nil, errors.Annotate(err, "to closing builder").Err()
	}
	result := &bqpb.StatusRow{
		TreeName:       s.TreeName,
		Message:        s.Message,
		CreateTime:     timestamppb.New(s.CreateTime),
		Status:         toBQStatusString(s.GeneralStatus),
		CreateUser:     toBQCreateUser(s.CreateUser),
		ClosingBuilder: closingBuilder,
	}
	return result, nil
}

func toBQStatusString(state v1.GeneralState) string {
	switch state {
	case v1.GeneralState_OPEN:
		return "open"
	case v1.GeneralState_CLOSED:
		return "closed"
	case v1.GeneralState_THROTTLED:
		return "throttled"
	case v1.GeneralState_MAINTENANCE:
		return "maintenance"
	default:
		return ""
	}
}

// If this is a service account, return as-is.
// Otherwise, just return "user" or empty string if there is no data.
func toBQCreateUser(createUser string) string {
	if strings.HasSuffix(createUser, "gserviceaccount.com") {
		return createUser
	}
	if createUser == "" {
		return ""
	}
	return "user"
}

func toClosingBuilder(closingBuilderName string) (*bqpb.Builder, error) {
	if closingBuilderName == "" {
		return nil, nil
	}
	re := regexp.MustCompile(`^` + status.BuilderNameExpression + `$`)
	matches := re.FindStringSubmatch(closingBuilderName)
	if len(matches) != 4 {
		return nil, errors.Reason("invalid builder name %q", closingBuilderName).Err()
	}
	return &bqpb.Builder{
		Project: matches[1],
		Bucket:  matches[2],
		Builder: matches[3],
	}, nil
}
