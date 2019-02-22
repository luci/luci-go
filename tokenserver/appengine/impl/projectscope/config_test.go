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

package projectscope

import (
	"context"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectidentity"
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"

	. "github.com/smartystreets/goconvey/convey"
)

const fakeConfig = `
	projects {
		id: "id1"
		config_location {
			url: "https://some/repo"
			storage_type: GITILES
		}
		identity_config {
			service_account_email: "foo@bar.com"
		}
	}
	projects {
		id: "id2"
		config_location {
			url: "https://some/other/repo"
			storage_type: GITILES
		}
		identity_config {
			service_account_email: "bar@bar.com"
		}
	}
`

func TestRules(t *testing.T) {
	t.Parallel()

	ctx := auth.WithState(gaetesting.TestingContext(), &authtest.FakeState{
		Identity: "user:unused@example.com",
		FakeDB: authtest.FakeDB{
			"user:via-group1@robots.com":           []string{"account-group-1"},
			"user:via-group2@robots.com":           []string{"account-group-2"},
			"user:via-both@robots.com":             []string{"account-group-1", "account-group-2"},
			"user:via-group1-and-rule1@robots.com": []string{"account-group-1"},
			"user:via-group1-and-rule2@robots.com": []string{"account-group-1"},
		},
	})
	storage := projectidentity.ProjectIdentities(ctx)

	Convey("Loads", t, func() {
		cfg, err := loadConfig(ctx, fakeConfig)
		So(err, ShouldBeNil)
		So(cfg, ShouldNotBeNil)

		expected := map[string]string{
			"id1": "foo@bar.com",
			"id2": "bar@bar.com",
		}

		for project, email := range expected {
			identity, err := storage.LookupByProject(ctx, project)
			So(err, ShouldBeNil)
			So(identity, ShouldResemble, &projectidentity.ProjectIdentity{
				Project: project,
				Email:   email,
			})
		}

	})

}

func loadConfig(ctx context.Context, text string) (*Rules, error) {
	cfg := &config.ProjectsCfg{}
	err := proto.UnmarshalText(text, cfg)
	if err != nil {
		return nil, err
	}
	rules, err := importIdentities(ctx, policy.ConfigBundle{projectsCfg: cfg}, "fake-revision")
	if err != nil {
		return nil, err
	}
	return rules.(*Rules), nil
}
