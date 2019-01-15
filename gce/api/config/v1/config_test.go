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

package config

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	Convey("validate", t, func() {
		c := &validation.Context{Context: context.Background()}
		k := stringset.New(0)

		Convey("empty", func() {
			cfg := &Config{}
			cfg.Validate(c, k)
			So(c.Finalize(), ShouldBeNil)
		})

		Convey("unknown kind", func() {
			k.Add("known")
			cfg := &Config{
				Kind: "unknown",
			}
			cfg.Validate(c, k)
			So(c.Finalize(), ShouldErrLike, "unknown kind")
		})
	})
}
