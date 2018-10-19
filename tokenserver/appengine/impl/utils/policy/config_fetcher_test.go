// Copyright 2017 The LUCI Authors.
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

package policy

import (
	"context"
	"testing"

	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfigFetcher(t *testing.T) {
	t.Parallel()

	Convey("Fetches bunch of configs", t, func() {
		c := gaetesting.TestingContext()
		c = prepareServiceConfig(c, map[string]string{
			"abc.cfg": "seconds: 12345",
			"def.cfg": "seconds: 67890",
		})

		f := luciConfigFetcher{}
		ts := timestamp.Timestamp{} // using timestamp as guinea pig proto message

		So(f.FetchTextProto(c, "missing", &ts), ShouldEqual, config.ErrNoConfig)
		So(f.Revision(), ShouldEqual, "")

		So(f.FetchTextProto(c, "abc.cfg", &ts), ShouldBeNil)
		So(ts.Seconds, ShouldEqual, 12345)
		So(f.FetchTextProto(c, "def.cfg", &ts), ShouldBeNil)
		So(ts.Seconds, ShouldEqual, 67890)

		So(f.Revision(), ShouldEqual, "20a03c9df37ce4413c01b580a8ae48a0aafce038")
	})

	Convey("Revision changes midway", t, func() {
		base := gaetesting.TestingContext()

		f := luciConfigFetcher{}
		ts := timestamp.Timestamp{}

		c1 := prepareServiceConfig(base, map[string]string{
			"abc.cfg": "seconds: 12345",
		})
		So(f.FetchTextProto(c1, "abc.cfg", &ts), ShouldBeNil)
		So(ts.Seconds, ShouldEqual, 12345)

		c2 := prepareServiceConfig(base, map[string]string{
			"def.cfg": "seconds: 12345",
		})
		So(f.FetchTextProto(c2, "def.cfg", &ts), ShouldErrLike,
			`expected config "def.cfg" to be at rev f4b3d57ff7afec86b3850f40713008ff4a494bda, `+
				`but got 32c954f97d0dae611fff581a39bca4a56686285b`)
	})
}

func prepareServiceConfig(c context.Context, configs map[string]string) context.Context {
	return testconfig.WithCommonClient(c, memory.New(map[config.Set]memory.Files{
		config.Set("services/" + info.AppID(c)): configs,
	}))
}
