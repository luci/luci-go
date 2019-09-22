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

package seccfg

import (
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSecurityConfig(t *testing.T) {
	Convey("Empty", t, func() {
		cfg, err := Parse(nil)
		So(err, ShouldBeNil)
		So(cfg.IsInternalService("something.example.com"), ShouldBeFalse)
	})

	Convey("IsInternalService works", t, func() {
		blob, _ := proto.Marshal(&protocol.SecurityConfig{
			InternalServiceRegexp: []string{
				`(.*-dot-)?i1\.example\.com`,
				`(.*-dot-)?i2\.example\.com`,
			},
		})
		cfg, err := Parse(blob)
		So(err, ShouldBeNil)

		So(cfg.IsInternalService("i1.example.com"), ShouldBeTrue)
		So(cfg.IsInternalService("i2.example.com"), ShouldBeTrue)
		So(cfg.IsInternalService("abc-dot-i1.example.com"), ShouldBeTrue)
		So(cfg.IsInternalService("external.example.com"), ShouldBeFalse)
		So(cfg.IsInternalService("something-i1.example.com"), ShouldBeFalse)
		So(cfg.IsInternalService("i1.example.com-something"), ShouldBeFalse)
	})
}
