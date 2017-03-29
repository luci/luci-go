// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"testing"

	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/clock/testclock"
	memcfg "github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/testconfig"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"github.com/luci/luci-go/server/auth/identity"

	"golang.org/x/net/context"

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
	c := memory.UseWithAppID(context.Background(), "dev~luci-milo")
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)

	if *generate {
		for _, tc := range testCases {
			fmt.Printf("Generating expectations for %s/%s\n", tc.builder, tc.build)
			build, err := build(c, "debug", tc.builder, tc.build)
			if err != nil {
				panic(fmt.Errorf("Could not run build() for %s/%s: %s", tc.builder, tc.build, err))
			}
			buildJSON, err := json.MarshalIndent(build, "", "  ")
			if err != nil {
				panic(fmt.Errorf("Could not JSON marshal %s/%s: %s", tc.builder, tc.build, err))
			}
			fname := fmt.Sprintf("%s.%d.build.json", tc.builder, tc.build)
			fpath := path.Join("expectations", fname)
			err = ioutil.WriteFile(fpath, []byte(buildJSON), 0644)
			if err != nil {
				panic(fmt.Errorf("Encountered error while trying to write to %s: %s", fpath, err))
			}
		}
		return
	}

	Convey(`A test Environment`, t, func() {
		c = testconfig.WithCommonClient(c, memcfg.New(bbAclConfigs))
		c = auth.WithState(c, &authtest.FakeState{
			Identity:       identity.AnonymousIdentity,
			IdentityGroups: []string{"all"},
		})

		for _, tc := range testCases {
			Convey(fmt.Sprintf("Test Case: %s/%s", tc.builder, tc.build), func() {
				build, err := build(c, "debug", tc.builder, tc.build)
				So(err, ShouldBeNil)
				fname := fmt.Sprintf("%s.%d.build.json", tc.builder, tc.build)
				So(build, shouldMatchExpectationsFor, fname)
			})
		}

		Convey(`Disallow anonomyous users from accessing internal builds`, func() {
			ds.Put(c, &buildbotBuild{
				Master:      "fake",
				Buildername: "fake",
				Number:      1,
				Internal:    true,
			})
			_, err := getBuild(c, "fake", "fake", 1)
			So(err, ShouldResemble, errNotAuth)
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

var bbAclConfigs = map[string]memcfg.ConfigSet{
	"services/luci-milo": {
		"settings.cfg": internalConfig,
	},
}
