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

package main

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
)

func getBQClient(ctx context.Context, luciProject string) (client *bigquery.Client, err error) {
	tr, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(luciProject), auth.WithScopes(bigquery.Scope))
	if err != nil {
		return nil, err
	}

	client, err = bigquery.NewClient(ctx, bqExport.Project, option.WithHTTPClient(&http.Client{
		Transport: tr,
	}))
	return
}

func checkBQTableExistence(ctx context.Context, client *bigquery.Client, bqExport *pb.BigQueryExport) (bool, error) {
	d := client.Dataset(bqExport.Dataset)
	// Check the existence of table.
	_, err := d.Table(bqExport.Table).Metadata(ctx)
	if ae, ok := err.(*googleapi.Error); ok && ae.Code == http.StatusNotFound {
		// Table doesn't exist.
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func createBQTable(ctx context.Context, client *bigquery.Client, bqExport *pb.BigQueryExport) error {
	t := client.Dataset(bqExport.Dataset).Table(bqExport.Table)
	err := t.Create(ctx, nil)
	if ae, ok := err.(*googleapi.Error); ok && ae.Code == http.StatusConflict {
		// Table already exist. This is fine.
		return nil
	}
	if err != nil {
		return err
	}
	logging.Infof(ctx, "Created BigQuery table %s.%s.%s", bqExport.Project, bqExport.Dataset, bqExport.Table)
	return nil
}

// ExportToBigQuery export test results of an invocation to a BigQuery table.
func ExportToBigQuery(ctx context.Context, luciProject string, invID string, bqExport *pb.BigQueryExport) error {
	client, err := getBQClient(ctx, luciProject)
	if err != nil {
		return err
	}
	defer client.Close()

	tableExist, err := checkBQTableExistence(ctx, client, bqExport)
	if err != nil {
		return err
	}
	if !tableExist {
		if err = createBQTable(ctx, client, bqExport); err != nil {
			return err
		}
	}
	return nil
}
