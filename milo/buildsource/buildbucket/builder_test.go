// Copyright 2016 The LUCI Authors.
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

package buildbucket

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/milo/common"

	. "github.com/smartystreets/goconvey/convey"
)

var generate = flag.Bool("test.generate", false, "Generate expectations instead of running tests.")

func TestBuilder(t *testing.T) {
	t.Parallel()

	testCases := []struct{ bucket, builder string }{
		{"master.tryserver.infra", "InfraPresubmit.Swarming"},
	}

	Convey("Builder", t, func() {
		c := gaetesting.TestingContextWithAppID("luci-milo-dev")
		c, _ = testclock.UseTime(c, testclock.TestRecentTimeUTC)
		c = testconfig.WithCommonClient(c, memcfg.New(bktConfigFull))
		// Update the service config so that the settings are loaded.
		_, err := common.UpdateServiceConfig(c)
		So(err, ShouldBeNil)

		for _, tc := range testCases {
			tc := tc
			Convey(fmt.Sprintf("%s:%s", tc.bucket, tc.builder), func() {
				expectationFilePath := filepath.Join("expectations", tc.bucket, tc.builder+".json")
				err := os.MkdirAll(filepath.Dir(expectationFilePath), 0777)
				So(err, ShouldBeNil)

				actual, err := GetBuilder(c, tc.bucket, tc.builder, 20)
				So(err, ShouldBeNil)
				actualJSON, err := json.MarshalIndent(actual, "", "  ")
				So(err, ShouldBeNil)

				if *generate {
					err := ioutil.WriteFile(expectationFilePath, actualJSON, 0777)
					So(err, ShouldBeNil)
				} else {
					expectedJSON, err := ioutil.ReadFile(expectationFilePath)
					So(err, ShouldBeNil)
					So(string(actualJSON), ShouldEqual, string(expectedJSON))
				}
			})
		}
	})
}

var bktConfig = `
buildbucket: {
	host: "debug"
	project: "debug"
}
`

var bktConfigFull = map[string]memcfg.ConfigSet{
	"services/luci-milo-dev": {
		"settings.cfg": bktConfig,
	},
}
