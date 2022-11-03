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
	"sync"
	"testing"
	"time"

	gax "github.com/googleapis/gax-go/v2"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSecretManagerSource(t *testing.T) {
	t.Parallel()

	Convey("With SecretManagerStore", t, func() {
		ctx := context.Background()
		ctx = gologger.StdConfig.Use(ctx)
		ctx = logging.SetLevel(ctx, logging.Debug)
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(123)))
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// Machinery to advance fake time in a lock step with MaintenanceLoop.
		ticks := make(chan bool)
		tc.SetTimerCallback(func(d time.Duration, _ clock.Timer) {
			// Do not block clock.After call itself, just delay advancing of the
			// clock until `ticks` is signaled.
			go func() {
				select {
				case do := <-ticks:
					if do {
						tc.Add(d)
					}
				case <-ctx.Done():
				}
			}()
		})

		// Handle reports from MaintenanceLoop.
		events := make(chan string)
		expectChecked := func(secret string) {
			So(<-events, ShouldEqual, "checking")
			So(<-events, ShouldEqual, "checked "+secret)
		}
		expectReloaded := func(secret string) {
			So(<-events, ShouldEqual, "checking")
			So(<-events, ShouldEqual, "reloaded "+secret)
		}
		expectSleeping := func() {
			So(<-events, ShouldEqual, "sleeping")
		}
		expectFullSleep := func(dur string) {
			expectSleeping()
			ticks <- true // advance clock in the timer
			So(<-events, ShouldEqual, "slept "+dur)
		}
		expectWoken := func(afterSleep bool) {
			So(<-events, ShouldEqual, "woken")
			if afterSleep {
				ticks <- false // the timer was aborted, don't advance clock in it
			}
			So(<-events, ShouldEqual, "checking")
		}

		gsm := secretManagerMock{}

		sm := SecretManagerStore{
			CloudProject:        "fake-project",
			AccessSecretVersion: gsm.AccessSecretVersion,
			testingEvents:       events,
		}
		go sm.MaintenanceLoop(ctx)
		So(<-events, ShouldEqual, "checking") // wait until it blocks

		Convey("normalizeName sm://<secret>", func() {
			name, err := sm.normalizeName("sm://secret")
			So(err, ShouldBeNil)
			So(name, ShouldEqual, "sm://fake-project/secret")
		})

		Convey("readSecret devsecret", func() {
			s, err := sm.readSecret(ctx, "devsecret://YWJj")
			So(err, ShouldBeNil)
			So(s, ShouldResemble, &trackedSecret{
				name: "devsecret://YWJj",
				value: Secret{
					Active: []byte("abc"),
				},
			})
		})

		Convey("readSecret devsecret-text", func() {
			s, err := sm.readSecret(ctx, "devsecret-text://abc")
			So(err, ShouldBeNil)
			So(s, ShouldResemble, &trackedSecret{
				name: "devsecret-text://abc",
				value: Secret{
					Active: []byte("abc"),
				},
			})
		})

		Convey("readSecret sm://<project>/<secret>", func() {
			gsm.createVersion("project", "secret", "zzz")

			s, err := sm.readSecret(ctx, "sm://project/secret")
			So(err, ShouldBeNil)
			So(s.name, ShouldEqual, "sm://project/secret")
			So(s.value, ShouldResemble, Secret{Active: []byte("zzz")})
			So(s.versions, ShouldEqual, [2]int64{1, 0})
			So(s.nextReload.IsZero(), ShouldBeFalse)
		})

		Convey("readSecret with prev version", func() {
			gsm.createVersion("project", "secret", "old")
			gsm.createVersion("project", "secret", "new")

			s, err := sm.readSecret(ctx, "sm://project/secret")
			So(err, ShouldBeNil)
			So(s.value, ShouldResemble, Secret{
				Active: []byte("new"),
				Passive: [][]byte{
					[]byte("old"),
				},
			})
			So(s.versions, ShouldEqual, [2]int64{2, 1})
		})

		Convey("readSecret with prev version disabled", func() {
			ref := gsm.createVersion("project", "secret", "old")
			gsm.createVersion("project", "secret", "new")
			gsm.disableVersion(ref)

			s, err := sm.readSecret(ctx, "sm://project/secret")
			So(err, ShouldBeNil)
			So(s.value, ShouldResemble, Secret{Active: []byte("new")})
			So(s.versions, ShouldEqual, [2]int64{2, 0})
		})

		Convey("readSecret with prev version deleted", func() {
			ref := gsm.createVersion("project", "secret", "old")
			gsm.createVersion("project", "secret", "new")
			gsm.deleteVersion(ref)

			s, err := sm.readSecret(ctx, "sm://project/secret")
			So(err, ShouldBeNil)
			So(s.value, ShouldResemble, Secret{Active: []byte("new")})
			So(s.versions, ShouldEqual, [2]int64{2, 0})
		})

		Convey("Derived secrets work", func() {
			var (
				ver1 = []byte{56, 113, 147, 97, 153, 240, 138, 213, 51, 101, 163, 53, 195, 45, 143, 253}
				ver2 = []byte{246, 39, 65, 71, 93, 43, 95, 59, 139, 134, 84, 40, 226, 44, 2, 47}
			)

			gsm.createVersion("project", "secret", "zzz")

			expectSleeping()
			So(sm.LoadRootSecret(ctx, "sm://project/secret"), ShouldBeNil)
			expectWoken(false)

			s1, err := sm.RandomSecret(ctx, "name")
			So(err, ShouldBeNil)
			So(s1, ShouldResemble, Secret{Active: ver1})

			rotated := make(chan struct{})
			sm.AddRotationHandler(ctx, "sm://project/secret", func(_ context.Context, s Secret) {
				close(rotated)
			})

			// Rotate the secret and make sure the change is picked up.
			gsm.createVersion("project", "secret", "xxx")
			expectFullSleep("2h16m23s")
			expectReloaded("sm://project/secret")
			<-rotated

			// Random secrets are rotated too.
			s2, err := sm.RandomSecret(ctx, "name")
			So(err, ShouldBeNil)
			So(s2, ShouldResemble, Secret{
				Active:  ver2,
				Passive: [][]byte{ver1},
			})
		})

		Convey("Stored secrets OK", func() {
			gsm.createVersion("project", "secret", "v1")

			rotated := make(chan struct{})
			sm.AddRotationHandler(ctx, "sm://project/secret", func(_ context.Context, s Secret) {
				rotated <- struct{}{}
			})

			expectSleeping()
			s, err := sm.StoredSecret(ctx, "sm://project/secret")
			So(err, ShouldBeNil)
			So(s, ShouldResemble, Secret{Active: []byte("v1")})
			expectWoken(false)

			// Rotate the secret and make sure the change is picked up when expected.
			gsm.createVersion("project", "secret", "v2")
			expectFullSleep("2h16m23s")
			expectReloaded("sm://project/secret")

			s, err = sm.StoredSecret(ctx, "sm://project/secret")
			So(err, ShouldBeNil)
			So(s, ShouldResemble, Secret{
				Active: []byte("v2"),
				Passive: [][]byte{
					[]byte("v1"),
				},
			})

			<-rotated // doesn't hang
		})

		Convey("Stored secrets exponential back-off", func() {
			gsm.createVersion("project", "secret", "v1")

			expectSleeping()
			s, err := sm.StoredSecret(ctx, "sm://project/secret")
			So(err, ShouldBeNil)
			So(s, ShouldResemble, Secret{Active: []byte("v1")})
			expectWoken(false)

			// Rotate the secret and "break" the backend.
			gsm.createVersion("project", "secret", "v2")
			gsm.setError(status.Errorf(codes.Internal, "boom"))

			// Attempts to do a regular update first.
			expectFullSleep("2h16m23s")
			expectChecked("sm://project/secret")

			// Notices the error and starts checking more often.
			expectFullSleep("2s")
			expectChecked("sm://project/secret")
			expectFullSleep("6s")
			expectChecked("sm://project/secret")

			// "Fix" the backend.
			gsm.setError(nil)

			// The updated eventually succeeds and returns to the slow schedule.
			expectFullSleep("14s")
			expectReloaded("sm://project/secret")
			expectFullSleep("2h17m45s")
			expectChecked("sm://project/secret")
		})

		Convey("Stored secrets priority queue", func() {
			gsm.createVersion("project", "secret1", "v1")
			gsm.createVersion("project", "secret2", "v1")

			// Load the first one and let it be for a while to advance time.
			expectSleeping()
			sm.StoredSecret(ctx, "sm://project/secret1")
			expectWoken(false)
			expectFullSleep("2h16m23s")
			expectChecked("sm://project/secret1")

			// Load the second one, it wakes up the maintenance loop, but it finds
			// there's nothing to reload yet.
			expectSleeping()
			sm.StoredSecret(ctx, "sm://project/secret2")
			expectWoken(true)

			// Starts waking up periodically to update one secret or another. Checks
			// for secret1 and secret2 happened to be bunched relatively close to
			// one another (~7m).
			expectFullSleep("3h47m31s")
			expectChecked("sm://project/secret2")
			expectFullSleep("6m39s")
			expectChecked("sm://project/secret1")
			expectFullSleep("2h17m45s")
			expectChecked("sm://project/secret1")
			expectFullSleep("12m48s")
			expectChecked("sm://project/secret2")
		})
	})
}

type secretManagerMock struct {
	m        sync.Mutex
	err      error
	versions map[string]string // full ref => a value or "" if disabled
	latest   map[string]int    // latest ref => version number
}

func (sm *secretManagerMock) createVersion(project, name, value string) string {
	sm.m.Lock()
	defer sm.m.Unlock()

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
	sm.m.Lock()
	defer sm.m.Unlock()

	sm.versions[ref] = ""
}

func (sm *secretManagerMock) deleteVersion(ref string) {
	sm.m.Lock()
	defer sm.m.Unlock()

	delete(sm.versions, ref)
}

func (sm *secretManagerMock) setError(err error) {
	sm.m.Lock()
	defer sm.m.Unlock()
	sm.err = err
}

func (sm *secretManagerMock) AccessSecretVersion(_ context.Context, req *secretmanagerpb.AccessSecretVersionRequest, _ ...gax.CallOption) (*secretmanagerpb.AccessSecretVersionResponse, error) {
	sm.m.Lock()
	defer sm.m.Unlock()

	if sm.err != nil {
		return nil, sm.err
	}

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
