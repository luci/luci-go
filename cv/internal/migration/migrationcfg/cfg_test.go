// Copyright 2021 The LUCI Authors.
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

package migrationcfg

import (
	"strings"
	"testing"

	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIsCVInCharge(t *testing.T) {
	t.Parallel()

	Convey("IsCVInChargeOfSomething works", t, func() {
		run := func(project string, settings *migrationpb.Settings, appID string) bool {
			ct := cvtesting.Test{AppID: appID}
			ctx, cancel := ct.SetUp()
			defer cancel()

			So(srvcfg.SetTestMigrationConfig(ctx, settings), ShouldBeNil)

			res, err := IsCVInChargeOfSomething(ctx, project)
			So(err, ShouldBeNil)
			return res
		}

		Convey("Good config", func() {
			settings := &migrationpb.Settings{
				ApiHosts: []*migrationpb.Settings_ApiHost{
					{
						Host:                 "cv.appspot.com",
						Prod:                 true,
						ProjectRegexp:        []string{"prod-.+", "dev-and-prod"},
						ProjectRegexpExclude: []string{".+-403"},
					},
					{
						Host:                 "cv2.appspot.com",
						Prod:                 true,
						ProjectRegexp:        []string{"prod-enabled-2"},
						ProjectRegexpExclude: []string{},
					},
					{
						Host:                 "cv-dev.appspot.com",
						ProjectRegexp:        []string{"dev-.+"},
						ProjectRegexpExclude: []string{},
					},
					{
						Host:                 "cv-dev2.appspot.com",
						ProjectRegexp:        []string{"dev-.+"},
						ProjectRegexpExclude: []string{"dev-not-2"},
					},
				},
				UseCvRuns: &migrationpb.Settings_UseCVRuns{
					ProjectRegexp:        []string{"prod-enabled-.+", "dev-.+"},
					ProjectRegexpExclude: []string{".+-404"},
				},
			}

			// project -> CV app managing it or "" if none.
			expected := map[string]string{
				"prod-enabled-1":    "cv",
				"prod-enabled-403":  "",       // excluded by `cv` excludes it
				"prod-enabled-2":    "",       // both `cv` and `cv2` match it
				"prod-enabled-404":  "",       // excluded by UseCvRuns.
				"dev-and-prod":      "cv",     // matches only 1 prod host
				"dev-1":             "",       // matches both dev hosts
				"dev-not-2":         "cv-dev", // matches just 1 dev host
				"arbitrary-project": "",
			}

			for project, expApp := range expected {
				found := false
				for _, h := range settings.GetApiHosts() {
					app := strings.TrimSuffix(h.GetHost(), ".appspot.com")
					res := run(project, settings, app)
					found = found || (app == expApp)
					So(res, ShouldEqual, app == expApp)
				}
				// Ensure expected table app is correct if not "".
				So(found, ShouldEqual, expApp != "")
			}
		})

		Convey("Bad include regexp is ignorable", func() {
			settings := &migrationpb.Settings{
				ApiHosts: []*migrationpb.Settings_ApiHost{
					{
						Host:                 "cv.appspot.com",
						Prod:                 true,
						ProjectRegexp:        []string{`invalid\K`, "incl.+"},
						ProjectRegexpExclude: []string{"ex.+"},
					},
				},
				UseCvRuns: &migrationpb.Settings_UseCVRuns{
					ProjectRegexp:        []string{`invalid\Z`, "incl.+"},
					ProjectRegexpExclude: []string{"ex.+"},
				},
			}
			So(run("included", settings, "cv"), ShouldBeTrue)
			So(run("unsure", settings, "cv"), ShouldBeFalse)
			So(run("excluded", settings, "cv"), ShouldBeFalse)
		})
		Convey("Bad exclude regexp excludes everything", func() {
			settings := &migrationpb.Settings{
				ApiHosts: []*migrationpb.Settings_ApiHost{
					{
						Host:                 "cv.appspot.com",
						Prod:                 true,
						ProjectRegexp:        []string{"incl.+"},
						ProjectRegexpExclude: []string{`bad\K`, "exc.+"},
					},
				},
				UseCvRuns: &migrationpb.Settings_UseCVRuns{
					ProjectRegexp:        []string{"incl.+"},
					ProjectRegexpExclude: []string{`bad\K`, "exc.+"},
				},
			}
			So(run("unsure", settings, "cv"), ShouldBeFalse)
			So(run("excluded", settings, "cv"), ShouldBeFalse)
		})
	})
}
