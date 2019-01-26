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

package projectscope

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"

	. "github.com/smartystreets/goconvey/convey"
)

const fakeConfig = `
services {
	service: "scheduler"
  cloud_project_id: "scheduler-project-1234"
  max_validity_duration: 1800
}
services {
	service: "buildbucket"
  cloud_project_id: "buildbucket-project-23434"
  max_validity_duration: 3600
}
services {
	service: "scheduler"
  cloud_project_id: "scheduler-project"
}
`

type testFetcher struct {
	Config string
}

func (s *testFetcher) FetchTextProto(c context.Context, path string, out proto.Message) error {
	cfg := out.(*admin.ProjectScopedServiceAccounts)
	err := proto.UnmarshalText(s.Config, cfg)
	if err != nil {
		return err
	}
	return nil
}

func TestConfig(t *testing.T) {
	t.Parallel()

	ctx := auth.WithState(context.Background(), &authtest.FakeState{
		Identity: "user:unused@example.com",
		FakeDB: authtest.FakeDB{
			"user:via-group1@robots.com":           []string{"account-group-1"},
			"user:via-group2@robots.com":           []string{"account-group-2"},
			"user:via-both@robots.com":             []string{"account-group-1", "account-group-2"},
			"user:via-group1-and-rule1@robots.com": []string{"account-group-1"},
			"user:via-group1-and-rule2@robots.com": []string{"account-group-1"},
		},
	})

	Convey("Config loads", t, func() {
		cfg, err := loadConfig(ctx, fakeConfig)
		So(err, ShouldBeNil)
		So(cfg, ShouldNotBeNil)

		Convey("Rule matching works", func() {
			rule, err := cfg.MatchingRules(ctx, "scheduler")
			So(err, ShouldBeNil)
			So(rule, ShouldResemble, []*Rule{{
				CloudProjectID: "scheduler-project",
				Rule: &admin.ServiceConfig{
					Service:             "scheduler",
					CloudProjectId:      "scheduler-project",
					MaxValidityDuration: 0,
				},
				Revision: "fake-revision",
			}})
		})

		Convey("makeRule works", func() {
			configs := map[string]struct {
				Config       *admin.ServiceConfig
				ExpectedErr  error
				ExpectedRule *Rule
			}{
				"happy": {
					Config: &admin.ServiceConfig{
						Service:        "foo",
						CloudProjectId: "foo-project",
					},
					ExpectedErr: nil,
					ExpectedRule: &Rule{
						CloudProjectID: "foo-project",
						Rule: &admin.ServiceConfig{
							Service:        "foo",
							CloudProjectId: "foo-project",
						},
						Revision: "fake-revision",
					},
				},
				"invalid": {
					Config: &admin.ServiceConfig{
						Service:        "",
						CloudProjectId: "",
					},
					ExpectedErr:  errors.New("service cannot be empty"),
					ExpectedRule: nil,
				},
			}

			for _, v := range configs {
				rule, err := makeRule(ctx, v.Config, "fake-revision")
				if v.ExpectedErr != nil {
					So(err, ShouldNotBeNil)
				} else {
					So(err, ShouldBeNil)
				}
				So(rule, ShouldResemble, v.ExpectedRule)
			}
		})

		Convey("SetupConfigValidation works", func() {

		})
	})

	Convey("Config fetching", t, func() {
		cfgBundle, err := fetchConfigs(ctx, &testFetcher{Config: fakeConfig})
		So(err, ShouldBeNil)
		So(cfgBundle, ShouldNotBeNil)
	})

}

func loadConfig(ctx context.Context, text string) (*Rules, error) {
	cfg := &admin.ProjectScopedServiceAccounts{}
	err := proto.UnmarshalText(text, cfg)
	if err != nil {
		return nil, err
	}
	rules, err := prepareRules(ctx, policy.ConfigBundle{projectScopedServiceAccountsCfg: cfg}, "fake-revision")
	if err != nil {
		return nil, err
	}
	return rules.(*Rules), nil
}
