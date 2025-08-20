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
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	rdbcfg "go.chromium.org/luci/resultdb/internal/config"
	configpb "go.chromium.org/luci/resultdb/proto/config"
)

var textPBMultiline = prototext.MarshalOptions{
	Multiline: true,
}

type projectConfigsKeyType struct{}

var projectConfigsKey = projectConfigsKeyType{}

func getConfigs(ctx context.Context) map[config.Set]cfgmem.Files {
	if cfgs, ok := ctx.Value(projectConfigsKey).(map[config.Set]cfgmem.Files); ok {
		return cfgs
	}
	return make(map[config.Set]cfgmem.Files)
}

func setConfigs(ctx context.Context, cfgs map[config.Set]cfgmem.Files) context.Context {
	return context.WithValue(ctx, projectConfigsKey, cfgs)
}

func getProjectConfig(ctx context.Context, t testing.TB, project string) *configpb.ProjectConfig {
	t.Helper()

	configs := getConfigs(ctx)
	cfgSet := config.Set(fmt.Sprintf("projects/%s", project))
	files, _ := configs[cfgSet]
	if files == nil {
		files = make(cfgmem.Files)
		configs[cfgSet] = files
	}

	var pcfg configpb.ProjectConfig
	if content, ok := files["${appid}.cfg"]; ok {
		if err := prototext.Unmarshal([]byte(content), &pcfg); err != nil {
			t.Fatalf("failed to unmarshal project config: %s", err)
		}
	} else {
		pcfg = *rdbcfg.CreatePlaceholderProjectConfig()
	}
	return &pcfg
}

func setProjectConfig(ctx context.Context, t testing.TB, project string, pcfg *configpb.ProjectConfig) context.Context {
	t.Helper()

	configs := getConfigs(ctx)
	cfgSet := config.Set(fmt.Sprintf("projects/%s", project))
	files, _ := configs[cfgSet]
	if files == nil {
		files = make(cfgmem.Files)
		configs[cfgSet] = files
	}

	files["${appid}.cfg"] = textPBMultiline.Format(pcfg)

	ctx = setConfigs(ctx, configs)
	ctx = cfgclient.Use(ctx, cfgmem.New(configs))
	err := rdbcfg.UpdateProjects(ctx)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).CatchupIndexes()

	return ctx
}

// ClearProjectConfig removes the configuration for a project.
func ClearProjectConfig(ctx context.Context, t testing.TB, project string) context.Context {
	t.Helper()

	configs := getConfigs(ctx)
	cfgSet := config.Set(fmt.Sprintf("projects/%s", project))
	delete(configs, cfgSet)

	ctx = setConfigs(ctx, configs)
	ctx = cfgclient.Use(ctx, cfgmem.New(configs))
	err := rdbcfg.UpdateProjects(ctx)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).CatchupIndexes()

	return ctx
}

// TestProjectConfigContext returns a context to be used in project config
// related tests.
func TestProjectConfigContext(ctx context.Context, t testing.TB, project, user, bucket, instance string) context.Context {
	t.Helper()
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
	ctx = memory.Use(ctx)
	ctx = caching.WithEmptyProcessCache(ctx)
	ctx = SetGCSAllowedBuckets(ctx, t, project, user, bucket)
	ctx = SetRBEAllowedInstances(ctx, t, project, user, instance)
	return ctx
}

// SetGCSAllowedBuckets sets the GCS allow list for a user in a project.
func SetGCSAllowedBuckets(ctx context.Context, t testing.TB, project, user string, buckets ...string) context.Context {
	pcfg := getProjectConfig(ctx, t, project)
	var rule *configpb.GcsAllowList
	for _, r := range pcfg.GcsAllowList {
		// Match only by user, this is sufficient for our tests.
		if len(r.Users) == 1 && r.Users[0] == user {
			rule = r
			break
		}
	}

	// If no rule for the user, create one.
	if rule == nil {
		rule = &configpb.GcsAllowList{Users: []string{user}}
		pcfg.GcsAllowList = append(pcfg.GcsAllowList, rule)
	}
	rule.Buckets = buckets
	return setProjectConfig(ctx, t, project, pcfg)
}

// SetRBEAllowedInstances sets the RBE allow list for a user in a project.
func SetRBEAllowedInstances(ctx context.Context, t testing.TB, project, user string, instances ...string) context.Context {
	pcfg := getProjectConfig(ctx, t, project)
	var rule *configpb.RbeAllowList
	for _, r := range pcfg.RbeAllowList {
		// Match only by user, this is sufficient for our tests.
		if len(r.Users) == 1 && r.Users[0] == user {
			rule = r
			break
		}
	}

	// If no rule for the user, create one.
	if rule == nil {
		rule = &configpb.RbeAllowList{Users: []string{user}}
		pcfg.RbeAllowList = append(pcfg.RbeAllowList, rule)
	}
	rule.Instances = instances
	return setProjectConfig(ctx, t, project, pcfg)
}
