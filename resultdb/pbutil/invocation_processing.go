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

package pbutil

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// ValidateBigQueryExport returns a non-nil error if bqExport is determined to
// be invalid.
func ValidateBigQueryExport(bqExport *pb.BigQueryExport) error {
	switch {
	case bqExport.Project == "":
		return errors.Annotate(unspecified(), "project").Err()
	case bqExport.Dataset == "":
		return errors.Annotate(unspecified(), "dataset").Err()
	case bqExport.Table == "":
		return errors.Annotate(unspecified(), "table").Err()
	case bqExport.GetTestResults() == nil:
		return errors.Annotate(unspecified(), "test_results").Err()
	}

	if bqExport.TestResults.GetPredicate() == nil {
		return nil
	}

	if err := ValidateTestResultPredicate(bqExport.TestResults.Predicate); err != nil {
		return errors.Annotate(err, "test_results: predicate").Err()
	}

	return nil
}

// CheckBQTableExistence checks if the BQ table exists.
func CheckBQTableExistence(ctx context.Context, luciProject string, bqExport *pb.BigQueryExport) error {
	tr, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(luciProject), auth.WithScopes(bigquery.Scope))
	if err != nil {
		return err
	}

	client, err := bigquery.NewClient(ctx, bqExport.Project, option.WithHTTPClient(&http.Client{
		Transport: tr,
	}))
	if err != nil {
		return err
	}
	defer client.Close()

	d := client.Dataset(bqExport.Dataset)
	// Check the existence of table.
	_, err = d.Table(bqExport.Table).Metadata(ctx)
	if ae, ok := err.(*googleapi.Error); ok && ae.Code == http.StatusNotFound {
		// Table doesn't exist.
		return errors.Reason("BigQuery table %s/%s/%s not exist", bqExport.Project, bqExport.Dataset, bqExport.Table).Err()
	}
	if err != nil {
		return err
	}

	return nil
}
