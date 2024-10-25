// Copyright 2015 The LUCI Authors.
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

package grpccode

import (
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/grpc/grpcutil"
)

// ShouldBe checks that the grpcutil.Code of the error matches `code`.
func ShouldBe(code codes.Code) comparison.Func[error] {
	return func(err error) *failure.Summary {
		actual := grpcutil.Code(err)
		if actual == code {
			return nil
		}
		return comparison.NewSummaryBuilder("grpccode.ShouldBe").
			Because("Error had incorrect gRPC code").
			Actual(actual).
			Expected(code).
			AddFindingf("err.Error()", err.Error()).
			Summary
	}
}
