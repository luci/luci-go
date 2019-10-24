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

package span

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// ReadTestResult reads specified TestResult within the transaction.
// If the TestResult does not exist, the returned error is annotated with
// NotFound GRPC code.
func ReadTestResult(ctx context.Context, txn Txn, invID, testPath, resultID string) (*pb.TestResult, error) {
	tr := &pb.TestResult{Name: pbutil.TestResultName(invID, testPath, resultID)}

	var unexpected bool
	var micros int64
	err := ReadRow(ctx, txn, "TestResults", spanner.Key{invID, testPath, resultID}, map[string]interface{}{
		"TestPath": &tr.TestPath,
		"ResultId": &tr.ResultId,
		"ExtraVariantPairs": &tr.ExtraVariantPairs,
		"IsUnexpected": &unexpected,
		"Status": &tr.Status,
		"SummaryMarkdown": &tr.SummaryMarkdown,
		"StartTime": &tr.StartTime,
		"RunDurationUsec": &micros,
		"Tags": &tr.Tags,
		"InputArtifacts": &tr.InputArtifacts,
		"OutputArtifacts": &tr.OutputArtifacts,
	})
	if err != nil {
		return nil, err
	}

	tr.Expected = !unexpected
	if micros > 0 {
		tr.Duration = ptypes.DurationProto(time.Duration(1e3*micros))
	}

	return tr, nil
}
