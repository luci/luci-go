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
	"testing"

	durationpb "github.com/golang/protobuf/ptypes/duration"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateMaxStaleness(t *testing.T) {
	t.Parallel()
	Convey(`ValidateMaxStaleness`, t, func() {
		Convey(`max_staleness`, func() {
			Convey(`lower boundary`, func() {
				err := ValidateMaxStaleness(&durationpb.Duration{Seconds: -1})
				So(err, ShouldErrLike, `must between 0 and 30m, inclusive`)
			})
			Convey(`upper boundary`, func() {
				err := ValidateMaxStaleness(&durationpb.Duration{Seconds: 10000})
				So(err, ShouldErrLike, `must between 0 and 30m, inclusive`)
			})
		})
	})
}
