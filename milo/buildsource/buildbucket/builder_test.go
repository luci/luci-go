// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbucket

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/common/clock/testclock"
	memcfg "github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/testconfig"
	"github.com/luci/luci-go/milo/common"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

var generate = flag.Bool("test.generate", false, "Generate expectations instead of running tests.")

func TestBuilder(t *testing.T) {
	t.Parallel()

	testCases := []struct{ bucket, builder string }{
		{"master.tryserver.infra", "InfraPresubmit"},
		{"master.tryserver.infra", "InfraPresubmit.Swarming"},
	}

	Convey("Builder", t, func() {
		c := memory.UseWithAppID(context.Background(), "luci-milo-dev")
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

				actual, err := builderImpl(c,
					builderQuery{
						Bucket:  tc.bucket,
						Builder: tc.builder,
						Limit:   0,
					})
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
