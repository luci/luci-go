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

package changepoints

import (
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// TestVariantBranch represents one row in the TestVariantBranch spanner table.
// See go/luci-test-variant-analysis-design for details.
type TestVariantBranch struct {
	// IsNew is a boolean to denote if the TestVariantBranch is new or already
	// existed in Spanner.
	// It is used for reducing the number of mutations. For example, the Variant
	// field is only inserted once.
	IsNew                  bool
	Project                string
	TestID                 string
	VariantHash            string
	GitReferenceHash       []byte
	Variant                *pb.Variant
	InputBuffer            *InputBuffer
	RecentChangepointCount int64
	// TODO (nqmtuan): Add output buffer.
}

// InsertToInputBuffer inserts data of a new test variant into the input
// buffer.
func (tvb *TestVariantBranch) InsertToInputBuffer(pv PositionVerdict) {
	tvb.InputBuffer.InsertVerdict(pv)
}
