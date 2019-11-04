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
	"strconv"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/span"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

func insertTestResult(invID span.InvocationID, tr *pb.TestResult, i int) (*spanner.Mutation, error) {
	trMap := map[string]interface{}{
		"InvocationId": invID,
		"TestPath":     tr.TestPath,
		"ResultId":     strconv.Itoa(i),

		"ExtraVariantPairs": tr.GetExtraVariantPairs(),

		"CommitTimestamp": spanner.CommitTimestamp,

		"Status":          tr.Status,
		"SummaryMarkdown": tr.SummaryMarkdown,
		"StartTime":       tr.StartTime,
		"RunDurationUsec": span.ToMicros(tr.Duration),
		"Tags":            tr.Tags,

		"InputArtifacts":  tr.InputArtifacts,
		"OutputArtifacts": tr.OutputArtifacts,
	}

	// Populate IsUnexpected /only/ if true, to keep the index thin.
	if !tr.Expected {
		trMap["IsUnexpected"] = true
	}

	return span.InsertMap("TestResults", trMap), nil
}
