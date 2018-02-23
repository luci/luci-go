// Copyright 2015 The LUCI Authors.
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

package swarming

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	memcfg "go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/gae/impl/memory"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/milo/buildsource/swarming/testdata"
)

var generate = flag.Bool("test.generate", false, "Generate expectations instead of running tests.")

func load(name string) ([]byte, error) {
	filename := filepath.Join("expectations", name)
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

	if *generate {
		c := context.Background()
		// This is one hour after the start timestamp in the sample test data.
		c, _ = testclock.UseTime(c, time.Date(2016, time.March, 14, 11, 0, 0, 0, time.UTC))
		c = memory.UseWithAppID(c, "dev~luci-milo")
		c = testconfig.WithCommonClient(c, memcfg.New(testdata.AclConfigs))
		c = auth.WithState(c, &authtest.FakeState{
			Identity:       "user:alicebob@google.com",
			IdentityGroups: []string{"all", "googlers"},
		})

		for _, tc := range testdata.GetTestCases(".") {
			fmt.Printf("Generating expectations for %s\n", tc)

			c := tc.InjectLogdogClient(c)
			build, err := SwarmingBuildImpl(c, tc, tc.Name)
			if err != nil {
				panic(fmt.Errorf("Could not run swarmingBuildImpl for %s: %s", tc, err))
			}
			buildJSON, err := json.MarshalIndent(build, "", "  ")
			if err != nil {
				panic(fmt.Errorf("Could not JSON marshal %s: %s", tc.Name, err))
			}
			filename := filepath.Join("expectations", tc.Name+".json")
			err = ioutil.WriteFile(filename, []byte(buildJSON), 0644)
			if err != nil {
				panic(fmt.Errorf("Encountered error while trying to write to %s: %s", filename, err))
			}
		}
		return
	}

	Convey(`A test Environment`, t, func() {
		c := context.Background()
		// This is one hour after the start timestamp in the sample test data.
		c, _ = testclock.UseTime(c, time.Date(2016, time.March, 14, 11, 0, 0, 0, time.UTC))
		c = memory.UseWithAppID(c, "dev~luci-milo")
		c = testconfig.WithCommonClient(c, memcfg.New(testdata.AclConfigs))
		c = auth.WithState(c, &authtest.FakeState{
			Identity:       "user:alicebob@google.com",
			IdentityGroups: []string{"all", "googlers"},
		})

		for _, tc := range testdata.GetTestCases(".") {
			Convey(fmt.Sprintf("Test Case: %s", tc.Name), func() {
				c := tc.InjectLogdogClient(c)

				// Special case: The build-internal test case to check that ACLs should fail.
				if tc.Name == "build-internal" {
					Convey("Should fail", func() {
						c := auth.WithState(c, &authtest.FakeState{
							Identity:       identity.AnonymousIdentity,
							IdentityGroups: []string{"all"},
						})
						_, err := SwarmingBuildImpl(c, tc, tc.Name)
						So(err.Error(), ShouldResemble, "Not a Milo Job or access denied")
					})
				}

				build, err := SwarmingBuildImpl(c, tc, tc.Name)
				So(err, ShouldBeNil)
				So(build, shouldMatchExpectationsFor, tc.Name+".json")
			})
		}
	})
}
