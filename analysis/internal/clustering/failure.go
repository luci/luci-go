// Copyright 2022 The LUCI Authors.
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

package clustering

import (
	"google.golang.org/protobuf/proto"

	cpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// Failure captures the minimal information required to cluster a failure.
// This is a subset of the information captured by LUCI Analysis for failures.
type Failure struct {
	// The name of the test that failed.
	TestID string
	// The failure reason explaining the reason why the test failed.
	Reason *pb.FailureReason
}

// FailureFromProto extracts failure information relevant for clustering from
// a LUCI Analysis failure proto.
func FailureFromProto(f *cpb.Failure) *Failure {
	result := &Failure{
		TestID: f.TestId,
	}
	if f.FailureReason != nil {
		result.Reason = proto.Clone(f.FailureReason).(*pb.FailureReason)
	}
	return result
}

// FailuresFromProtos extracts failure information relevant for clustering
// from a set of LUCI Analysis failure protos.
func FailuresFromProtos(protos []*cpb.Failure) []*Failure {
	result := make([]*Failure, len(protos))
	for i, p := range protos {
		result[i] = FailureFromProto(p)
	}
	return result
}
