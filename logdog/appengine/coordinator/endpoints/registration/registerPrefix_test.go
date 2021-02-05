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

package registration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/registration/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRegisterPrefix(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install(true)
		c, fb := featureBreaker.FilterRDS(c, nil)

		// Mock random number generator so we can predict secrets.
		c = cryptorand.MockForTest(c, 0)
		randSecret := []byte{
			250, 18, 249, 42, 251, 224, 15, 133, 8, 208, 232, 59,
			171, 156, 248, 206, 191, 66, 226, 94, 139, 20, 234, 252,
			129, 234, 224, 208, 15, 44, 173, 228, 193, 124, 22, 209,
		}
		nonce := []byte{
			20, 57, 74, 77, 46, 8, 108, 144, 0, 31, 138, 106, 154, 46, 215, 125, 142,
			23, 246, 192, 81, 14, 187, 119, 241, 171, 82, 85, 182, 233, 7, 119,
		}

		svr := New()

		const project = "proj-foo"
		req := logdog.RegisterPrefixRequest{
			Project:    project,
			Prefix:     "testing/prefix",
			SourceInfo: []string{"unit test"},
			OpNonce:    nonce,
		}
		pfx := &coordinator.LogPrefix{ID: coordinator.LogPrefixID(types.StreamName(req.Prefix))}

		Convey(`Authorization rules`, func() {
			// "proj-exclusive" has restricted legacy ACLs, see coordinatorTest.
			const project = "proj-exclusive"
			req.Project = project

			const (
				User = "user:caller@example.com"
				Anon = identity.AnonymousIdentity
			)

			const (
				NoRealm        = ""
				AllowedRealm   = "allowed"
				ForbiddenRealm = "forbidden"
			)

			const (
				AllowLegacy  = true
				ForbidLegacy = false
			)

			realm := func(short string) string {
				return realms.Join(project, short)
			}

			cases := []struct {
				enforce     bool              // is enforce_realms_in ON or OFF
				ident       identity.Identity // who's making the call
				realm       string            // the realm in the RPC
				allowLegacy bool              // mock legacy ACLs to allow access
				code        codes.Code        // the expected gRPC code
			}{
				// Enforcement is on, legacy ACLs are ignored.
				{true, User, NoRealm, AllowLegacy, codes.InvalidArgument},
				{true, User, NoRealm, ForbidLegacy, codes.InvalidArgument},
				{true, User, AllowedRealm, AllowLegacy, codes.OK},
				{true, User, AllowedRealm, ForbidLegacy, codes.OK},
				{true, User, ForbiddenRealm, AllowLegacy, codes.PermissionDenied},
				{true, User, ForbiddenRealm, ForbidLegacy, codes.PermissionDenied},

				// Enforcement is off, have a fallback on legacy ACLs.
				{false, User, NoRealm, AllowLegacy, codes.OK},
				{false, User, NoRealm, ForbidLegacy, codes.PermissionDenied},
				{false, User, AllowedRealm, AllowLegacy, codes.OK},
				{false, User, AllowedRealm, ForbidLegacy, codes.OK},
				{false, User, ForbiddenRealm, AllowLegacy, codes.OK},
				{false, User, ForbiddenRealm, ForbidLegacy, codes.PermissionDenied},

				// Permission denied for anon => Unauthenticated.
				{true, Anon, ForbiddenRealm, AllowLegacy, codes.Unauthenticated},
				{false, Anon, NoRealm, ForbidLegacy, codes.Unauthenticated},
			}

			for i, test := range cases {
				Convey(fmt.Sprintf("Case #%d", i), func() {
					mocks := []authtest.MockedDatum{
						authtest.MockPermission(test.ident, realm(AllowedRealm), coordinator.PermLogsCreate),
					}
					if test.enforce {
						mocks = append(mocks, authtest.MockRealmData(
							realm(realms.RootRealm),
							&protocol.RealmData{EnforceInService: []string{env.ServiceID}},
						))
					}
					if test.allowLegacy {
						mocks = append(mocks, authtest.MockMembership(test.ident, "auth"))
					}

					c := auth.WithState(c, &authtest.FakeState{
						Identity: test.ident,
						FakeDB:   authtest.NewFakeDB(mocks...),
					})

					req.Realm = test.realm
					_, err := svr.RegisterPrefix(c, &req)
					So(status.Code(err), ShouldEqual, test.code)
				})
			}
		})

		Convey(`Will register a new prefix.`, func() {
			resp, err := svr.RegisterPrefix(c, &req)
			So(err, ShouldBeNil)
			So(resp, ShouldResemble, &logdog.RegisterPrefixResponse{
				LogBundleTopic: "projects/logdog-app-id/topics/test-topic",
				Secret:         randSecret,
			})

			ct.WithProjectNamespace(c, project, func(c context.Context) {
				So(ds.Get(c, pfx), ShouldBeNil)
			})
			So(pfx, ShouldResemble, &coordinator.LogPrefix{
				Schema:  coordinator.CurrentSchemaVersion,
				ID:      pfx.ID,
				Prefix:  "testing/prefix",
				Created: ds.RoundTime(clock.Now(c)),
				Source:  []string{"unit test"},
				Secret:  randSecret,
				OpNonce: nonce,

				// 24 hours is default service prefix expiration.
				Expiration: ds.RoundTime(clock.Now(c).Add(24 * time.Hour)),
			})

			Convey(`Will refuse to register it again without nonce.`, func() {
				req.OpNonce = nil
				_, err := svr.RegisterPrefix(c, &req)
				So(err, ShouldBeRPCAlreadyExists)
			})

			Convey(`Is happy to return the same data if the nonce matches.`, func() {
				resp, err := svr.RegisterPrefix(c, &req)
				So(err, ShouldBeNil)
				So(resp, ShouldResemble, &logdog.RegisterPrefixResponse{
					LogBundleTopic: "projects/logdog-app-id/topics/test-topic",
					Secret:         randSecret,
				})
			})

			Convey(`Expires the nonce after 15 minutes.`, func() {
				env.Clock.Add(coordinator.RegistrationNonceTimeout + time.Second)
				_, err := svr.RegisterPrefix(c, &req)
				So(err, ShouldBeRPCAlreadyExists)
			})
		})

		Convey(`Uses the correct prefix expiration`, func() {

			Convey(`When service, project, and request have expiration, chooses smallest.`, func() {
				env.ModProjectConfig(c, project, func(pcfg *svcconfig.ProjectConfig) {
					pcfg.PrefixExpiration = google.NewDuration(time.Hour)
				})
				req.Expiration = google.NewDuration(time.Minute)

				_, err := svr.RegisterPrefix(c, &req)
				So(err, ShouldBeNil)

				ct.WithProjectNamespace(c, project, func(c context.Context) {
					So(ds.Get(c, pfx), ShouldBeNil)
				})
				So(pfx.Expiration, ShouldResemble, clock.Now(c).Add(time.Minute))
			})

			Convey(`When service, and project have expiration, chooses smallest.`, func() {
				env.ModProjectConfig(c, project, func(pcfg *svcconfig.ProjectConfig) {
					pcfg.PrefixExpiration = google.NewDuration(time.Hour)
				})

				_, err := svr.RegisterPrefix(c, &req)
				So(err, ShouldBeNil)

				ct.WithProjectNamespace(c, project, func(c context.Context) {
					So(ds.Get(c, pfx), ShouldBeNil)
				})
				So(pfx.Expiration, ShouldResemble, clock.Now(c).Add(time.Hour))
			})

			Convey(`When no expiration is defined, failed with internal error.`, func() {
				env.ModServiceConfig(c, func(cfg *svcconfig.Config) {
					cfg.Coordinator.PrefixExpiration = nil
				})

				_, err := svr.RegisterPrefix(c, &req)
				So(err, ShouldBeRPCInvalidArgument, "no prefix expiration defined")
			})
		})

		Convey(`Will fail to register the prefix if Put is broken.`, func() {
			fb.BreakFeatures(errors.New("test error"), "PutMulti")
			_, err := svr.RegisterPrefix(c, &req)
			So(err, ShouldBeRPCInternal)
		})
	})
}
