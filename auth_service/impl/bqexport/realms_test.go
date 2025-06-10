// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bqexport

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/bqpb"
	customerrors "go.chromium.org/luci/auth_service/impl/errors"
)

func TestBQRealms(t *testing.T) {
	t.Parallel()
	ftt.Run("parseRealms works", t, func(t *ftt.Test) {
		ctx := context.Background()
		testRev := int64(1001)
		testTime := timestamppb.New(
			time.Date(2020, time.August, 16, 15, 20, 0, 0, time.UTC))
		t.Run("empty Realms returns error", func(t *ftt.Test) {
			_, err := parseRealms(ctx, &protocol.AuthDB{}, testRev, testTime)
			assert.Loosely(t, err,
				should.ErrLike(customerrors.ErrAuthDBMissingRealms))
		})
		t.Run("realm rows are constructed", func(t *ftt.Test) {
			testAuthDB := &protocol.AuthDB{
				Realms: &protocol.Realms{
					Permissions: []*protocol.Permission{
						{Name: "luci.dev.p1"},
						{Name: "luci.dev.p2"},
						{Name: "luci.dev.p3"},
						{Name: "luci.dev.p4"},
					},
					Conditions: []*protocol.Condition{
						{
							Op: &protocol.Condition_Restrict{
								Restrict: &protocol.Condition_AttributeRestriction{
									Attribute: "x",
									Values:    []string{"4", "9", "16"},
								},
							},
						},
						{
							Op: &protocol.Condition_Restrict{
								Restrict: &protocol.Condition_AttributeRestriction{
									Attribute: "y",
									Values:    []string{"8", "27"},
								},
							},
						},
						{
							Op: &protocol.Condition_Restrict{
								Restrict: &protocol.Condition_AttributeRestriction{
									Attribute: "y",
									Values:    []string{"10"},
								},
							},
						},
					},
					Realms: []*protocol.Realm{
						{
							Name: "p:@root",
						},
						{
							Name: "p:r",
							Bindings: []*protocol.Binding{
								{
									Permissions: []uint32{0, 1},
									Principals:  []string{"group:gr1"},
								},
								{
									Permissions: []uint32{0, 1, 2},
									Principals:  []string{"group:gr3", "group:gr4"},
									Conditions:  []uint32{0, 1, 2},
								},
								{
									Permissions: []uint32{1, 2},
									Principals:  []string{"group:gr1", "group:gr2"},
								},
							},
						},
						{
							Name: "p:r2",
							Bindings: []*protocol.Binding{
								{
									Permissions: []uint32{2},
									Principals:  []string{"group:gr1"},
									Conditions:  []uint32{0},
								},
							},
						},
					},
				},
			}
			expected := []*bqpb.RealmRow{
				{
					Name:        "p:r",
					BindingId:   0,
					Permissions: []string{"luci.dev.p1", "luci.dev.p2"},
					Principals:  []string{"group:gr1"},
					AuthdbRev:   testRev,
					ExportedAt:  testTime,
				},
				{
					Name:        "p:r",
					BindingId:   1,
					Permissions: []string{"luci.dev.p1", "luci.dev.p2", "luci.dev.p3"},
					Principals:  []string{"group:gr3", "group:gr4"},
					Conditions: []string{
						"x==(16||4||9)", "y==(10)", "y==(27||8)"},
					AuthdbRev:  testRev,
					ExportedAt: testTime,
				},
				{
					Name:        "p:r",
					BindingId:   2,
					Permissions: []string{"luci.dev.p2", "luci.dev.p3"},
					Principals:  []string{"group:gr1", "group:gr2"},
					AuthdbRev:   testRev,
					ExportedAt:  testTime,
				},
				{
					Name:        "p:r2",
					BindingId:   0,
					Permissions: []string{"luci.dev.p3"},
					Principals:  []string{"group:gr1"},
					Conditions:  []string{"x==(16||4||9)"},
					AuthdbRev:   testRev,
					ExportedAt:  testTime,
				},
			}
			actual, err := parseRealms(ctx, testAuthDB, testRev, testTime)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(expected))
		})
	})
}

func TestExpandLatestRealms(t *testing.T) {
	t.Parallel()

	ftt.Run("expandLatestRoles works", t, func(t *ftt.Test) {
		ctx := context.Background()
		testTime := timestamppb.New(
			time.Date(2020, time.August, 16, 15, 20, 0, 0, time.UTC))

		realmsURL := "https://path.to.view/config/realms.cfg/123abc"
		testRealmsCfg := &realmsconf.RealmsCfg{
			Realms: []*realmsconf.Realm{
				{
					Name: "@project",
					Bindings: []*realmsconf.Binding{
						{
							Role:       "role/luci.projectUser",
							Principals: []string{"group:luci-trusted-projects"},
						},
					},
				},
				{
					Name: "@root",
					Bindings: []*realmsconf.Binding{
						{
							Role:       "role/luci.serviceTester",
							Principals: []string{"group:luci-service-testers"},
						},
					},
				},
				{
					Name: "ci",
					Bindings: []*realmsconf.Binding{
						{
							Role:       "role/luci.resourceUser",
							Principals: []string{"group:luci-resource-users"},
						},
					},
					Extends: []string{"unknown"},
				},
				{
					Name: "try",
					Bindings: []*realmsconf.Binding{
						{
							Role:       "role/luci.jobTriggerer",
							Principals: []string{"group:luci-job-triggerers"},
						},
					},
					Extends: []string{"ci"},
				},
			},
		}
		testRealms := map[string]*ViewableConfig[*realmsconf.RealmsCfg]{
			"chromium": {
				ViewURL: realmsURL,
				Config:  testRealmsCfg,
			},
		}

		actual, err := expandLatestRealms(ctx, testRealms, testTime)
		assert.Loosely(t, err, should.BeNil)
		expected := []*bqpb.RealmSourceRow{
			{
				Name:       "chromium:@project",
				Role:       "role/luci.projectUser",
				Source:     "chromium:@project",
				Principals: []string{"group:luci-trusted-projects"},
				Url:        realmsURL,
				ExportedAt: testTime,
			},
			{
				Name:       "chromium:@project",
				Role:       "role/luci.serviceTester",
				Source:     "chromium:@root",
				Principals: []string{"group:luci-service-testers"},
				Url:        realmsURL,
				ExportedAt: testTime,
			},
			{
				Name:       "chromium:@root",
				Role:       "role/luci.serviceTester",
				Source:     "chromium:@root",
				Principals: []string{"group:luci-service-testers"},
				Url:        realmsURL,
				ExportedAt: testTime,
			},
			{
				Name:       "chromium:ci",
				Role:       "role/luci.resourceUser",
				Source:     "chromium:ci",
				Principals: []string{"group:luci-resource-users"},
				Url:        realmsURL,
				ExportedAt: testTime,
			},
			{
				Name:       "chromium:ci",
				Role:       "role/luci.serviceTester",
				Source:     "chromium:@root",
				Principals: []string{"group:luci-service-testers"},
				Url:        realmsURL,
				ExportedAt: testTime,
			},
			{
				Name:       "chromium:try",
				Role:       "role/luci.jobTriggerer",
				Source:     "chromium:try",
				Principals: []string{"group:luci-job-triggerers"},
				Url:        realmsURL,
				ExportedAt: testTime,
			},
			{
				Name:       "chromium:try",
				Role:       "role/luci.resourceUser",
				Source:     "chromium:ci",
				Principals: []string{"group:luci-resource-users"},
				Url:        realmsURL,
				ExportedAt: testTime,
			},
			{
				Name:       "chromium:try",
				Role:       "role/luci.serviceTester",
				Source:     "chromium:@root",
				Principals: []string{"group:luci-service-testers"},
				Url:        realmsURL,
				ExportedAt: testTime,
			},
		}

		rowSorter := func(a, b *bqpb.RealmSourceRow) int {
			return strings.Compare(a.Name+a.Source+a.Role, b.Name+b.Source+b.Role)
		}

		// Sort the rows so we can easily compare to the expected value.
		slices.SortStableFunc(actual, rowSorter)
		slices.SortStableFunc(expected, rowSorter)
		assert.Loosely(t, actual, should.Match(expected))
	})
}
