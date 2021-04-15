// Copyright 2020 The LUCI Authors.
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

package secrets

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	gax "github.com/googleapis/gax-go/v2"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSecretManagerSource(t *testing.T) {
	t.Parallel()

	Convey("With SecretManagerStore", t, func() {
		ctx := mathrand.Set(context.Background(), rand.New(rand.NewSource(123)))
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		gsm := secretManagerMock{}

		sm := SecretManagerStore{
			CloudProject:        "fake-project",
			AccessSecretVersion: gsm.AccessSecretVersion,
		}

		Convey("normalizeName sm://<secret>", func() {
			name, err := sm.normalizeName("sm://secret")
			So(err, ShouldBeNil)
			So(name, ShouldEqual, "sm://fake-project/secret")
		})

		Convey("readSecret devsecret", func() {
			s, err := sm.readSecret(ctx, "devsecret://YWJj", false)
			So(err, ShouldBeNil)
			So(s, ShouldResemble, &trackedSecret{
				name: "devsecret://YWJj",
				value: Secret{
					Current: []byte("abc"),
				},
			})
		})

		Convey("readSecret devsecret-text", func() {
			s, err := sm.readSecret(ctx, "devsecret-text://abc", false)
			So(err, ShouldBeNil)
			So(s, ShouldResemble, &trackedSecret{
				name: "devsecret-text://abc",
				value: Secret{
					Current: []byte("abc"),
				},
			})
		})

		Convey("readSecret sm://<project>/<secret>", func() {
			gsm.createVersion("project", "secret", "zzz")

			s, err := sm.readSecret(ctx, "sm://project/secret", false)
			So(err, ShouldBeNil)
			So(s.name, ShouldEqual, "sm://project/secret")
			So(s.value, ShouldResemble, Secret{Current: []byte("zzz")})
			So(s.versions, ShouldEqual, [2]int64{1, 0})
			So(s.nextReload.IsZero(), ShouldBeFalse)
		})

		Convey("readSecret with prev version", func() {
			gsm.createVersion("project", "secret", "old")
			gsm.createVersion("project", "secret", "new")

			s, err := sm.readSecret(ctx, "sm://project/secret", true)
			So(err, ShouldBeNil)
			So(s.value, ShouldResemble, Secret{
				Current: []byte("new"),
				Previous: [][]byte{
					[]byte("old"),
				},
			})
			So(s.versions, ShouldEqual, [2]int64{2, 1})
		})

		Convey("readSecret with prev version disabled", func() {
			ref := gsm.createVersion("project", "secret", "old")
			gsm.createVersion("project", "secret", "new")
			gsm.disableVersion(ref)

			s, err := sm.readSecret(ctx, "sm://project/secret", true)
			So(err, ShouldBeNil)
			So(s.value, ShouldResemble, Secret{Current: []byte("new")})
			So(s.versions, ShouldEqual, [2]int64{2, 0})
		})

		Convey("readSecret with prev version deleted", func() {
			ref := gsm.createVersion("project", "secret", "old")
			gsm.createVersion("project", "secret", "new")
			gsm.deleteVersion(ref)

			s, err := sm.readSecret(ctx, "sm://project/secret", true)
			So(err, ShouldBeNil)
			So(s.value, ShouldResemble, Secret{Current: []byte("new")})
			So(s.versions, ShouldEqual, [2]int64{2, 0})
		})

		Convey("Derived secrets work", func() {
			var (
				ver1 = []byte{56, 113, 147, 97, 153, 240, 138, 213, 51, 101, 163, 53, 195, 45, 143, 253}
				ver2 = []byte{246, 39, 65, 71, 93, 43, 95, 59, 139, 134, 84, 40, 226, 44, 2, 47}
			)

			gsm.createVersion("project", "secret", "zzz")

			So(sm.LoadRootSecret(ctx, "sm://project/secret"), ShouldBeNil)

			s1, err := sm.RandomSecret(ctx, "name")
			So(err, ShouldBeNil)
			So(s1, ShouldResemble, Secret{Current: ver1})

			rotated := make(chan struct{})
			sm.AddRotationHandler(ctx, "sm://project/secret", func(context.Context, Secret) {
				close(rotated)
			})

			// Rotate the secret and make sure the change is picked up.
			gsm.createVersion("project", "secret", "xxx")
			tc.Add(reloadIntervalMax + time.Second)
			bctx, cancel := context.WithCancel(ctx)
			go sm.MaintenanceLoop(bctx)
			defer cancel()
			<-rotated

			// Random secrets are rotated too.
			s2, err := sm.RandomSecret(ctx, "name")
			So(err, ShouldBeNil)
			So(s2, ShouldResemble, Secret{
				Current:  ver2,
				Previous: [][]byte{ver1},
			})
		})
	})
}

type secretManagerMock struct {
	calls []string

	versions map[string]string // full ref => a value or "" if disabled
	latest   map[string]int    // latest ref => version number
}

func (sm *secretManagerMock) createVersion(project, name, value string) string {
	if sm.versions == nil {
		sm.versions = make(map[string]string, 1)
		sm.latest = make(map[string]int, 1)
	}

	latestRef := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", project, name)

	nextVer := sm.latest[latestRef] + 1
	versionRef := fmt.Sprintf("projects/%s/secrets/%s/versions/%d", project, name, nextVer)

	sm.versions[versionRef] = value
	sm.latest[latestRef] = nextVer

	return versionRef
}

func (sm *secretManagerMock) disableVersion(ref string) {
	sm.versions[ref] = ""
}

func (sm *secretManagerMock) deleteVersion(ref string) {
	delete(sm.versions, ref)
}

func (sm *secretManagerMock) AccessSecretVersion(_ context.Context, req *secretmanagerpb.AccessSecretVersionRequest, _ ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error) {
	sm.calls = append(sm.calls, req.Name)

	// Recognize ".../versions/latest" aliases.
	versionRef := req.Name
	if ver := sm.latest[versionRef]; ver != 0 {
		versionRef = strings.ReplaceAll(versionRef, "/versions/latest", fmt.Sprintf("/versions/%d", ver))
	}

	switch val, ok := sm.versions[versionRef]; {
	case !ok:
		return nil, status.Errorf(codes.NotFound, "no such version")
	case val == "":
		return nil, status.Errorf(codes.FailedPrecondition, "the version is disabled")
	default:
		return &secretmanagerpb.AccessSecretVersionResponse{
			Name:    versionRef,
			Payload: &secretmanagerpb.SecretPayload{Data: []byte(val)},
		}, nil
	}
}
