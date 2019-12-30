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

package testutil

import (
	"fmt"

	"github.com/smartystreets/assertions"
	"google.golang.org/grpc/codes"

	luciassertions "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/resultdb/internal/appstatus"
)

// ShouldHaveAppStatus assertions that error `actual` has an
// application-specific status and it matches the expectations.
// See ShouldBeStatusLike for the format of `expected`.
// See appstatus package for application-specific statuses.
func ShouldHaveAppStatus(actual interface{}, expected ...interface{}) string {
	// Special case for OK code.
	if len(expected) > 0 {
		if code, ok := expected[0].(codes.Code); ok && code == codes.OK {
			return assertions.ShouldBeNil(actual)
		}
	}

	if ret := assertions.ShouldImplement(actual, (*error)(nil)); ret != "" {
		return ret
	}
	actualStatus, ok := appstatus.FromError(actual.(error))
	if !ok {
		return fmt.Sprintf("expected error %q to have an explicit application status", actual)
	}

	return luciassertions.ShouldBeStatusLike(actualStatus, expected)
}
