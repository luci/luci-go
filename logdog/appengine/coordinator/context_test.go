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

package coordinator

import (
	"errors"
	"fmt"
	"testing"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/logdog/appengine/coordinator/config"
	"go.chromium.org/luci/luci_config/common/cfgtypes"
	"go.chromium.org/luci/luci_config/server/cfgclient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/identity"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type testServices struct {
	Services

	configErr error
	configs   map[cfgtypes.ProjectName]*svcconfig.ProjectConfig
}

func (s *testServices) ProjectConfig(c context.Context, project cfgtypes.ProjectName) (*svcconfig.ProjectConfig, error) {
	if err := s.configErr; err != nil {
		return nil, err
	}

	cfg, ok := s.configs[project]
	switch {
	case !ok:
		return nil, cfgclient.ErrNoConfig

	case cfg == nil:
		return nil, config.ErrInvalidConfig

	default:
		return cfg, nil
	}
}

func TestWithProjectNamespace(t *testing.T) {
	t.Parallel()

	Convey(`A testing environment`, t, func() {
		c := context.Background()
		c = memory.Use(c)

		// Fake authentication state.
		as := authtest.FakeState{
			IdentityGroups: []string{"all"},
		}
		c = auth.WithState(c, &as)

		// Fake service with fake project configs.
		svc := testServices{
			configs: map[cfgtypes.ProjectName]*svcconfig.ProjectConfig{
				"all-access": {
					ReaderAuthGroups: []string{"all"},
					WriterAuthGroups: []string{"all"},
				},
				"exclusive-access": {
					ReaderAuthGroups: []string{"auth"},
					WriterAuthGroups: []string{"auth"},
				},
			},
		}
		c = WithServices(c, &svc)

		Convey(`When using NamespaceAccessNoAuth with anonymous identity`, func() {
			So(auth.CurrentIdentity(c).Kind(), ShouldEqual, identity.Anonymous)

			Convey(`Can enter exclusive namespace`, func() {
				So(WithProjectNamespace(&c, "exclusive-access", NamespaceAccessNoAuth), ShouldBeNil)
				So(CurrentProject(c), ShouldEqual, "exclusive-access")
			})

			Convey(`Will fail to enter a namespace for a non-existent project with Unauthenticated.`, func() {
				So(WithProjectNamespace(&c, "does-not-exist", NamespaceAccessNoAuth), ShouldBeRPCUnauthenticated)
			})
		})

		Convey(`When using NamespaceAccessAllTesting with anonymous identity`, func() {
			So(auth.CurrentIdentity(c).Kind(), ShouldEqual, identity.Anonymous)

			Convey(`Can enter exclusive namespace`, func() {
				So(WithProjectNamespace(&c, "exclusive-access", NamespaceAccessAllTesting), ShouldBeNil)
				So(CurrentProject(c), ShouldEqual, "exclusive-access")
			})

			Convey(`Will fail to enter a namespace for a non-existent project.`, func() {
				So(WithProjectNamespace(&c, "does-not-exist", NamespaceAccessAllTesting), ShouldBeNil)
				So(CurrentProject(c), ShouldEqual, "does-not-exist")
			})
		})

		for _, tc := range []struct {
			testName string
			access   NamespaceAccessType
		}{
			{"READ", NamespaceAccessREAD},
			{"WRITE", NamespaceAccessWRITE},
		} {
			Convey(fmt.Sprintf(`When requesting %s access`, tc.testName), func() {

				Convey(`When logged in`, func() {
					id, err := identity.MakeIdentity("user:testing@example.com")
					if err != nil {
						panic(err)
					}
					as.Identity = id

					Convey(`Will successfully access public project.`, func() {
						So(WithProjectNamespace(&c, "all-access", tc.access), ShouldBeNil)
					})

					Convey(`When user is a member of exclusive group`, func() {
						as.IdentityGroups = append(as.IdentityGroups, "auth")

						Convey(`Can access exclusive namespace.`, func() {
							So(WithProjectNamespace(&c, "exclusive-access", tc.access), ShouldBeNil)
							So(CurrentProject(c), ShouldEqual, "exclusive-access")
						})

						Convey(`Will fail to access non-existent project with PermissionDenied.`, func() {
							So(WithProjectNamespace(&c, "does-not-exist", tc.access), ShouldBeRPCPermissionDenied)
						})
					})

					Convey(`Will fail to access exclusive project with PermissionDenied.`, func() {
						So(WithProjectNamespace(&c, "exclusive-access", tc.access), ShouldBeRPCPermissionDenied)
					})

					Convey(`Will fail to access non-existent project with PermissionDenied.`, func() {
						So(WithProjectNamespace(&c, "does-not-exist", tc.access), ShouldBeRPCPermissionDenied)
					})
				})

				Convey(`Will successfully access public project.`, func() {
					So(WithProjectNamespace(&c, "all-access", tc.access), ShouldBeNil)
				})

				Convey(`Will fail to access exclusive project with Unauthenticated.`, func() {
					So(WithProjectNamespace(&c, "exclusive-access", tc.access), ShouldBeRPCUnauthenticated)
				})

				Convey(`Will fail to access non-existent project with Unauthenticated.`, func() {
					So(WithProjectNamespace(&c, "does-not-exist", tc.access), ShouldBeRPCUnauthenticated)
				})

				Convey(`When config service returns an unexpected error`, func() {
					svc.configErr = errors.New("misc")

					for _, proj := range []cfgtypes.ProjectName{"all-access", "exclusive-access", "does-not-exist"} {
						Convey(fmt.Sprintf(`Will fail to access %q with Internal.`, proj), func() {
							So(WithProjectNamespace(&c, "all-access", tc.access), ShouldBeRPCInternal)
						})
					}
				})
			})
		}
	})
}
