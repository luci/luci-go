// Copyright 2020 The LUCI Authors.
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
	"flag"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/services/deriver"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func main() {
	bqTableFlag := flag.String(
		"derive-bigquery-table",
		"",
		`Name of the BigQuery table for result export. In the format of "<project>.<dataset>.<table>".`,
	)
	expectedTestResultsExpirationDays := flag.Int(
		"expected-results-expiration",
		60,
		"How many days to keep results for test variants with only expected results",
	)

	internal.Main(func(srv *server.Server) error {
		derivedInvBQTable, err := parseBQTable(*bqTableFlag)
		if err != nil {
			return err
		}

		deriver.InitServer(srv, deriver.Options{
			InvBQTable:                derivedInvBQTable,
			ExpectedResultsExpiration: time.Duration(*expectedTestResultsExpirationDays) * 24 * time.Hour,
		})
		return nil
	})
}

func parseBQTable(bqTable string) (*pb.BigQueryExport, error) {
	if bqTable == "" {
		return nil, errors.Reason("-derive-bigquery-table is missing").Err()
	}

	p := strings.Split(bqTable, ".")
	if len(p) != 3 || p[0] == "" || p[1] == "" || p[2] == "" {
		return nil, errors.Reason("invalid bq table %q", bqTable).Err()
	}

	return &pb.BigQueryExport{
		Project:     p[0],
		Dataset:     p[1],
		Table:       p[2],
		TestResults: &pb.BigQueryExport_TestResults{},
	}, nil
}
