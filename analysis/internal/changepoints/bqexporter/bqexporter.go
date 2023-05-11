// Copyright 2023 The LUCI Authors.
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

// Package bqexporter handles the export of test variant analysis results
// to BigQuery.
package bqexporter

import (
	"context"

	tvbr "go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
)

// ExportTestVariantBranch export test variant branch to BigQuery.
func ExportTestVariantBranch(ctx context.Context, tvb *tvbr.TestVariantBranch) error {
	// TODO (nqmtuan): Implement this.
	return nil
}
