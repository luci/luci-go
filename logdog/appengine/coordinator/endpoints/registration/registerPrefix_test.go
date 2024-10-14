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
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
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

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRegisterPrefix(t *testing.T) {
	t.Parallel()

	ftt.Run(`With a testing configuration`, t, func(t *ftt.Test) {
		c, env := ct.Install()
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

		const project = "some-project"
		const realm = "some-realm"

		env.AddProject(c, project)
		env.ActAsWriter(project, realm)

		req := logdog.RegisterPrefixRequest{
			Project:    project,
			Realm:      realm,
			Prefix:     "testing/prefix",
			SourceInfo: []string{"unit test"},
			OpNonce:    nonce,
		}
		pfx := &coordinator.LogPrefix{ID: coordinator.LogPrefixID(types.StreamName(req.Prefix))}

		svr := New()

		t.Run(`Authorization rules`, func(t *ftt.Test) {
			const (
				User   = "user:caller@example.com"
				Anon   = "anonymous:anonymous"
				Legacy = "user:legacy-caller@example.com"
			)
			const (
				NoRealm        = ""
				AllowedRealm   = "allowed"
				ForbiddenRealm = "forbidden"
			)

			realm := func(short string) string {
				return realms.Join(project, short)
			}
			authDB := authtest.NewFakeDB(
				authtest.MockPermission(User, realm(AllowedRealm), coordinator.PermLogsCreate),
				authtest.MockPermission(Legacy, realm(realms.LegacyRealm), coordinator.PermLogsCreate),
			)

			cases := []struct {
				ident identity.Identity // who's making the call
				realm string            // the realm in the RPC
				code  codes.Code        // the expected gRPC code
			}{
				{User, AllowedRealm, codes.OK},
				{User, ForbiddenRealm, codes.PermissionDenied},
				{Anon, ForbiddenRealm, codes.Unauthenticated},

				// Fallback to "@legacy" realm.
				{User, NoRealm, codes.PermissionDenied},
				{Legacy, NoRealm, codes.OK},
				{Anon, NoRealm, codes.Unauthenticated},
			}

			for i, test := range cases {
				t.Run(fmt.Sprintf("Case #%d", i), func(t *ftt.Test) {
					// Note: this overrides mocks set by ActAsWriter.
					c := auth.WithState(c, &authtest.FakeState{
						Identity: test.ident,
						FakeDB:   authDB,
					})
					req.Realm = test.realm
					_, err := svr.RegisterPrefix(c, &req)
					assert.Loosely(t, status.Code(err), should.Equal(test.code))
				})
			}
		})

		t.Run(`Will register a new prefix.`, func(t *ftt.Test) {
			resp, err := svr.RegisterPrefix(c, &req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Resemble(&logdog.RegisterPrefixResponse{
				LogBundleTopic: "projects/logdog-app-id/topics/test-topic",
				Secret:         randSecret,
			}))

			ct.WithProjectNamespace(c, project, func(c context.Context) {
				assert.Loosely(t, ds.Get(c, pfx), should.BeNil)
			})
			assert.Loosely(t, pfx, should.Resemble(&coordinator.LogPrefix{
				Schema:  coordinator.CurrentSchemaVersion,
				ID:      pfx.ID,
				Prefix:  "testing/prefix",
				Realm:   realms.Join(project, realm),
				Created: ds.RoundTime(clock.Now(c)),
				Source:  []string{"unit test"},
				Secret:  randSecret,
				OpNonce: nonce,

				// 24 hours is default service prefix expiration.
				Expiration: ds.RoundTime(clock.Now(c).Add(24 * time.Hour)),
			}))

			t.Run(`Will refuse to register it again without nonce.`, func(t *ftt.Test) {
				req.OpNonce = nil
				_, err := svr.RegisterPrefix(c, &req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCAlreadyExists)())
			})

			t.Run(`Is happy to return the same data if the nonce matches.`, func(t *ftt.Test) {
				resp, err := svr.RegisterPrefix(c, &req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Resemble(&logdog.RegisterPrefixResponse{
					LogBundleTopic: "projects/logdog-app-id/topics/test-topic",
					Secret:         randSecret,
				}))
			})

			t.Run(`Expires the nonce after 15 minutes.`, func(t *ftt.Test) {
				env.Clock.Add(coordinator.RegistrationNonceTimeout + time.Second)
				_, err := svr.RegisterPrefix(c, &req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCAlreadyExists)())
			})
		})

		t.Run(`Uses the correct prefix expiration`, func(t *ftt.Test) {

			t.Run(`When service, project, and request have expiration, chooses smallest.`, func(t *ftt.Test) {
				env.ModProjectConfig(c, project, func(pcfg *svcconfig.ProjectConfig) {
					pcfg.PrefixExpiration = durationpb.New(time.Hour)
				})
				req.Expiration = durationpb.New(time.Minute)

				_, err := svr.RegisterPrefix(c, &req)
				assert.Loosely(t, err, should.BeNil)

				ct.WithProjectNamespace(c, project, func(c context.Context) {
					assert.Loosely(t, ds.Get(c, pfx), should.BeNil)
				})
				assert.Loosely(t, pfx.Expiration, should.Resemble(clock.Now(c).Add(time.Minute)))
			})

			t.Run(`When service, and project have expiration, chooses smallest.`, func(t *ftt.Test) {
				env.ModProjectConfig(c, project, func(pcfg *svcconfig.ProjectConfig) {
					pcfg.PrefixExpiration = durationpb.New(time.Hour)
				})

				_, err := svr.RegisterPrefix(c, &req)
				assert.Loosely(t, err, should.BeNil)

				ct.WithProjectNamespace(c, project, func(c context.Context) {
					assert.Loosely(t, ds.Get(c, pfx), should.BeNil)
				})
				assert.Loosely(t, pfx.Expiration, should.Resemble(clock.Now(c).Add(time.Hour)))
			})

			t.Run(`When no expiration is defined, failed with internal error.`, func(t *ftt.Test) {
				env.ModServiceConfig(c, func(cfg *svcconfig.Config) {
					cfg.Coordinator.PrefixExpiration = nil
				})

				_, err := svr.RegisterPrefix(c, &req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("no prefix expiration defined"))
			})
		})

		t.Run(`Will fail to register the prefix if Put is broken.`, func(t *ftt.Test) {
			fb.BreakFeatures(errors.New("test error"), "PutMulti")
			_, err := svr.RegisterPrefix(c, &req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInternal)())
		})
	})
}
