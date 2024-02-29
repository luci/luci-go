// Copyright 2024 The LUCI Authors.
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

package swarmingimpl

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCancelTasksParse(t *testing.T) {
	t.Parallel()

	expectErr := func(argv []string, errLike string) {
		_, code, _, stderr := SubcommandTest(
			context.Background(),
			CmdCancelTasks,
			append([]string{"-server", "example.com"}, argv...),
			nil, nil,
		)
		So(code, ShouldEqual, 1)
		So(stderr, ShouldContainSubstring, errLike)
	}

	Convey(`Make sure that Parse fails with zero -limit.`, t, func() {
		expectErr([]string{"-limit", "0"}, "must be positive")
		expectErr([]string{"-limit", "-1"}, "must be positive")
	})

	Convey(`Make sure that tag is specified when limit is greater than default limit.`, t, func() {
		expectErr([]string{"-limit", "500"}, "cannot be larger than")
	})

}
