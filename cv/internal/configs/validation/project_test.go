// Copyright 2018 The LUCI Authors.
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

package validation

import (
	"context"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/config/validation"
	cfgpb "go.chromium.org/luci/cv/api/config/v2"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateProject(t *testing.T) {
	t.Parallel()

	Convey("ValidateProject works", t, func() {
		cfg := cfgpb.Config{}
		So(prototext.Unmarshal([]byte(validConfigTextPB), &cfg), ShouldBeNil)

		Convey("OK", func() {
			So(ValidateProject(&cfg), ShouldBeNil)
		})
		Convey("Warnings are OK", func() {
			cfg.GetConfigGroups()[0].GetVerifiers().GetTryjob().GetBuilders()[0].LocationRegexp = []string{"https://x.googlesource.com/my/repo/[+]/*.cpp"}
			So(ValidateProject(&cfg), ShouldBeNil)

			// Ensure this test doesn't bitrot and actually tests warnings.
			vctx := validation.Context{Context: context.Background()}
			validateProjectConfig(&vctx, &cfg)
			So(mustWarn(vctx.Finalize()), ShouldErrLike, "did you mean")
		})
		Convey("Error", func() {
			cfg.GetConfigGroups()[0].Name = "!invalid! name"
			So(ValidateProject(&cfg), ShouldErrLike, "must match")
		})
	})
}
