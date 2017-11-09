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

package buildbot

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/common/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	memcfg "go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
)

var generate = flag.Bool("test.generate", false, "Generate expectations instead of running tests.")

func load(name string) ([]byte, error) {
	filename := strings.Join([]string{"expectations", name}, "/")
	return ioutil.ReadFile(filename)
}

func shouldMatchExpectationsFor(actualContents interface{}, expectedFilename ...interface{}) string {
	refBuild, err := load(expectedFilename[0].(string))
	if err != nil {
		return fmt.Sprintf("Could not load %s: %s", expectedFilename[0], err.Error())
	}
	actualBuild, err := json.MarshalIndent(actualContents, "", "  ")
	return ShouldEqual(string(actualBuild), string(refBuild))

}

func TestBuild(t *testing.T) {
	t.Parallel()

	Convey(`TestBuild`, t, func() {
		c := memory.UseWithAppID(context.Background(), "dev~luci-milo")
		c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
		c = caching.WithRequestCache(c)

		if *generate {
			for _, tc := range TestCases {
				fmt.Printf("Generating expectations for %s/%d\n", tc.Builder, tc.Build)
				build, err := DebugBuild(c, ".", tc.Builder, tc.Build)
				So(err, ShouldBeNil)
				buildJSON, err := json.MarshalIndent(build, "", "  ")
				So(err, ShouldBeNil)
				fname := fmt.Sprintf("%s.%d.build.json", tc.Builder, tc.Build)
				fpath := filepath.Join("expectations", fname)
				So(ioutil.WriteFile(fpath, []byte(buildJSON), 0644), ShouldBeNil)
			}
			return
		}

		Convey(`A test Environment`, func() {
			c = testconfig.WithCommonClient(c, memcfg.New(bbACLConfigs))
			c = auth.WithState(c, &authtest.FakeState{
				Identity:       identity.AnonymousIdentity,
				IdentityGroups: []string{"all"},
			})
			c = caching.WithProcessCache(c, &lru.Cache{})

			for _, tc := range TestCases {
				Convey(fmt.Sprintf("Test Case: %s/%d", tc.Builder, tc.Build), func() {
					build, err := DebugBuild(c, ".", tc.Builder, tc.Build)
					So(err, ShouldBeNil)
					fname := fmt.Sprintf("%s.%d.build.json", tc.Builder, tc.Build)
					So(build, shouldMatchExpectationsFor, fname)
				})
			}

			Convey(`Disallow anonomyous users from accessing internal builds`, func() {
				importBuild(c, &buildbot.Build{
					Master:      "fake",
					Buildername: "fake",
					Number:      1,
					Internal:    true,
				})
				_, err := Build(c, "fake", "fake", 1)
				So(common.ErrorTag.In(err), ShouldEqual, common.CodeUnauthorized)
			})
		})
	})
}

var internalConfig = `
buildbot: {
	internal_reader: "googlers"
	public_subscription: "projects/luci-milo/subscriptions/buildbot-public"
	internal_subscription: "projects/luci-milo/subscriptions/buildbot-private"
}
`

var bbACLConfigs = map[string]memcfg.ConfigSet{
	"services/luci-milo": {
		"settings.cfg": internalConfig,
	},
}
