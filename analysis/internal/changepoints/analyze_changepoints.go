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

// Package changepoints handles change point detection and analysis.
// See go/luci-test-variant-analysis-design for details.
package changepoints

import (
	"context"

	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/common/logging"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

func Analyze(ctx context.Context, tvs []*rdbpb.TestVariant, payload *taskspb.IngestTestResults) error {
	// TODO (nqmtuan): Implement this.
	logging.Debugf(ctx, "Analyzing %d changepoints for build %d", len(tvs), payload.Build.Id)
	return nil
}
