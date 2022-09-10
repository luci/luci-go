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

package protowalk

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRequired(t *testing.T) {
	t.Parallel()

	Convey(`Required field check`, t, func() {
		msg := &Outer{
			SingleInner: &Inner{},
			MapInner: map[string]*Inner{
				"schwoot": {
					Deprecated:  "thing",
					SingleEmbed: &Inner_Embedded{},
					MultiEmbed: []*Inner_Embedded{
						{},
					},
				},
			},
			MultiDeprecated: []*Inner{
				{},
			},
		}

		res := Fields(msg, &RequiredProcessor{})
		So(res.Strings(), ShouldResemble, []string{
			`.req: required`,
			`.single_inner.req: required`,
			`.map_inner["schwoot"].req: required`,
			`.map_inner["schwoot"].single_embed.req: required`,
			`.map_inner["schwoot"].multi_embed[0].req: required`,
			`.multi_deprecated[0].req: required`,
		})
		So(res.Err(), ShouldErrLike, "required")
	})
}
