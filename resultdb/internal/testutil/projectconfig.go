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

package testutil

import (
	"context"
	"fmt"

	"github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	rdbcfg "go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/server/caching"
)

var textPBMultiline = prototext.MarshalOptions{
	Multiline: true,
}

// TestProjectConfigContext returns a context to be used in project config related tests.
func TestProjectConfigContext(ctx context.Context, project, user, bucket string) context.Context {
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
	ctx = memory.Use(ctx)
	ctx = caching.WithEmptyProcessCache(ctx)
	ctx = SetGCSAllowedBuckets(ctx, project, user, bucket)
	return ctx
}

// SetGCSAllowedBuckets overrides the only existing project-config
// GcsAllowlist entry to match the rule defined by project, realm, bucket
// and prefix.
func SetGCSAllowedBuckets(ctx context.Context, project, user, bucket string) context.Context {
	testProject := rdbcfg.CreatePlaceholderProjectConfig()
	convey.So(len(testProject.GcsAllowList), convey.ShouldEqual, 1)
	testProject.GcsAllowList[0].Users = []string{user}
	convey.So(len(testProject.GcsAllowList[0].Buckets), convey.ShouldEqual, 1)
	testProject.GcsAllowList[0].Buckets[0] = bucket

	cfgSet := config.Set(fmt.Sprintf("projects/%s", project))
	configs := map[config.Set]cfgmem.Files{
		cfgSet: {"${appid}.cfg": textPBMultiline.Format(testProject)},
	}

	ctx = cfgclient.Use(ctx, cfgmem.New(configs))
	err := rdbcfg.UpdateProjects(ctx)
	convey.So(err, convey.ShouldBeNil)
	datastore.GetTestable(ctx).CatchupIndexes()

	return ctx
}
