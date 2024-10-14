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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSecretManagerSource(t *testing.T) {
	t.Parallel()

	ftt.Run("With SecretManagerStore", t, func(t *ftt.Test) {
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
			assert.Loosely(t, <-events, should.Equal("checking"))
			assert.Loosely(t, <-events, should.Equal("checked "+secret))
		}
		expectReloaded := func(secret string) {
			assert.Loosely(t, <-events, should.Equal("checking"))
			assert.Loosely(t, <-events, should.Equal("reloaded "+secret))
		}
		expectSleeping := func() {
			assert.Loosely(t, <-events, should.Equal("sleeping"))
		}
		expectFullSleep := func(dur string) {
			expectSleeping()
			ticks <- true // advance clock in the timer
			assert.Loosely(t, <-events, should.Equal("slept "+dur))
		}
		expectWoken := func(afterSleep bool) {
			assert.Loosely(t, <-events, should.Equal("woken"))
			if afterSleep {
				ticks <- false // the timer was aborted, don't advance clock in it
			}
			assert.Loosely(t, <-events, should.Equal("checking"))
		}

		gsm := secretManagerMock{}

		sm := SecretManagerStore{
			CloudProject:        "fake-project",
			AccessSecretVersion: gsm.AccessSecretVersion,
			testingEvents:       events,
		}
		go sm.MaintenanceLoop(ctx)
		assert.Loosely(t, <-events, should.Equal("checking")) // wait until it blocks

		t.Run("normalizeName sm://<secret>", func(t *ftt.Test) {
			name, err := sm.normalizeName("sm://secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, name, should.Equal("sm://fake-project/secret"))
		})

		t.Run("readSecret devsecret", func(t *ftt.Test) {
			s, err := sm.readSecret(ctx, "devsecret://YWJj")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s, should.Resemble(&trackedSecret{
				name: "devsecret://YWJj",
				value: Secret{
					Active: []byte("abc"),
				},
			}))
		})

		t.Run("readSecret devsecret-text", func(t *ftt.Test) {
			s, err := sm.readSecret(ctx, "devsecret-text://abc")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s, should.Resemble(&trackedSecret{
				name: "devsecret-text://abc",
				value: Secret{
					Active: []byte("abc"),
				},
			}))
		})

		t.Run("readSecret sm://<project>/<secret>", func(t *ftt.Test) {
			gsm.createVersion("project", "secret", "zzz")

			s, err := sm.readSecret(ctx, "sm://project/secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s.name, should.Equal("sm://project/secret"))
			assert.Loosely(t, s.value, should.Resemble(Secret{Active: []byte("zzz")}))
			assert.Loosely(t, s.versionCurrent, should.Equal(1))
			assert.Loosely(t, s.versionPrevious, should.BeZero)
			assert.Loosely(t, s.versionNext, should.BeZero)
			assert.Loosely(t, s.nextReload.IsZero(), should.BeFalse)
		})

		t.Run("readSecret with all aliases", func(t *ftt.Test) {
			prev := gsm.createVersion("project", "secret", "prev")
			gsm.createVersion("project", "secret", "unreferenced 1")
			cur := gsm.createVersion("project", "secret", "cur")
			gsm.createVersion("project", "secret", "unreferenced 2")
			next := gsm.createVersion("project", "secret", "next")

			gsm.setAlias("current", cur)
			gsm.setAlias("previous", prev)
			gsm.setAlias("next", next)

			s, err := sm.readSecret(ctx, "sm://project/secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s.value, should.Resemble(Secret{
				Active: []byte("cur"),
				Passive: [][]byte{
					[]byte("prev"),
					[]byte("next"),
				},
			}))
			assert.Loosely(t, s.versionCurrent, should.Equal(3))
			assert.Loosely(t, s.versionPrevious, should.Equal(1))
			assert.Loosely(t, s.versionNext, should.Equal(5))
		})

		t.Run("readSecret without next", func(t *ftt.Test) {
			prev := gsm.createVersion("project", "secret", "prev")
			gsm.createVersion("project", "secret", "unreferenced 1")
			cur := gsm.createVersion("project", "secret", "cur")
			gsm.createVersion("project", "secret", "unreferenced 2")

			gsm.setAlias("current", cur)
			gsm.setAlias("previous", prev)

			s, err := sm.readSecret(ctx, "sm://project/secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s.value, should.Resemble(Secret{
				Active: []byte("cur"),
				Passive: [][]byte{
					[]byte("prev"),
				},
			}))
			assert.Loosely(t, s.versionCurrent, should.Equal(3))
			assert.Loosely(t, s.versionPrevious, should.Equal(1))
			assert.Loosely(t, s.versionNext, should.BeZero)
		})

		t.Run("readSecret with next disabled", func(t *ftt.Test) {
			prev := gsm.createVersion("project", "secret", "prev")
			gsm.createVersion("project", "secret", "unreferenced 1")
			cur := gsm.createVersion("project", "secret", "cur")
			gsm.createVersion("project", "secret", "unreferenced 2")
			next := gsm.createVersion("project", "secret", "next")

			gsm.setAlias("current", cur)
			gsm.setAlias("previous", prev)
			gsm.setAlias("next", next)

			gsm.disableVersion(next)

			s, err := sm.readSecret(ctx, "sm://project/secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s.value, should.Resemble(Secret{
				Active: []byte("cur"),
				Passive: [][]byte{
					[]byte("prev"),
				},
			}))
			assert.Loosely(t, s.versionCurrent, should.Equal(3))
			assert.Loosely(t, s.versionPrevious, should.Equal(1))
			assert.Loosely(t, s.versionNext, should.BeZero)
		})

		t.Run("readSecret with next set to current", func(t *ftt.Test) {
			prev := gsm.createVersion("project", "secret", "prev")
			gsm.createVersion("project", "secret", "unreferenced 1")
			cur := gsm.createVersion("project", "secret", "cur")
			gsm.createVersion("project", "secret", "unreferenced 2")

			gsm.setAlias("current", cur)
			gsm.setAlias("previous", prev)
			gsm.setAlias("next", cur)

			s, err := sm.readSecret(ctx, "sm://project/secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s.value, should.Resemble(Secret{
				Active: []byte("cur"),
				Passive: [][]byte{
					[]byte("prev"),
				},
			}))
			assert.Loosely(t, s.versionCurrent, should.Equal(3))
			assert.Loosely(t, s.versionPrevious, should.Equal(1))
			assert.Loosely(t, s.versionNext, should.Equal(3))
		})

		t.Run("readSecret with all aliases set to the same version", func(t *ftt.Test) {
			cur := gsm.createVersion("project", "secret", "cur")

			gsm.setAlias("current", cur)
			gsm.setAlias("previous", cur)
			gsm.setAlias("next", cur)

			s, err := sm.readSecret(ctx, "sm://project/secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s.value, should.Resemble(Secret{
				Active: []byte("cur"),
			}))
			assert.Loosely(t, s.versionCurrent, should.Equal(1))
			assert.Loosely(t, s.versionPrevious, should.Equal(1))
			assert.Loosely(t, s.versionNext, should.Equal(1))
		})

		t.Run("readSecret with prev version, legacy scheme", func(t *ftt.Test) {
			gsm.createVersion("project", "secret", "old")
			gsm.createVersion("project", "secret", "new")

			s, err := sm.readSecret(ctx, "sm://project/secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s.value, should.Resemble(Secret{
				Active: []byte("new"),
				Passive: [][]byte{
					[]byte("old"),
				},
			}))
			assert.Loosely(t, s.versionCurrent, should.Equal(2))
			assert.Loosely(t, s.versionPrevious, should.Equal(1))
			assert.Loosely(t, s.versionNext, should.BeZero)
		})

		t.Run("readSecret with prev version disabled, legacy scheme", func(t *ftt.Test) {
			ref := gsm.createVersion("project", "secret", "old")
			gsm.createVersion("project", "secret", "new")
			gsm.disableVersion(ref)

			s, err := sm.readSecret(ctx, "sm://project/secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s.value, should.Resemble(Secret{Active: []byte("new")}))
			assert.Loosely(t, s.versionCurrent, should.Equal(2))
			assert.Loosely(t, s.versionPrevious, should.BeZero)
			assert.Loosely(t, s.versionNext, should.BeZero)
		})

		t.Run("readSecret with prev version deleted, legacy scheme", func(t *ftt.Test) {
			ref := gsm.createVersion("project", "secret", "old")
			gsm.createVersion("project", "secret", "new")
			gsm.deleteVersion(ref)

			s, err := sm.readSecret(ctx, "sm://project/secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s.value, should.Resemble(Secret{Active: []byte("new")}))
			assert.Loosely(t, s.versionCurrent, should.Equal(2))
			assert.Loosely(t, s.versionPrevious, should.BeZero)
			assert.Loosely(t, s.versionNext, should.BeZero)
		})

		t.Run("Derived secrets work", func(t *ftt.Test) {
			var (
				ver1 = []byte{56, 113, 147, 97, 153, 240, 138, 213, 51, 101, 163, 53, 195, 45, 143, 253}
				ver2 = []byte{246, 39, 65, 71, 93, 43, 95, 59, 139, 134, 84, 40, 226, 44, 2, 47}
			)

			gsm.createVersion("project", "secret", "zzz")

			expectSleeping()
			assert.Loosely(t, sm.LoadRootSecret(ctx, "sm://project/secret"), should.BeNil)
			expectWoken(false)

			s1, err := sm.RandomSecret(ctx, "name")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s1, should.Resemble(Secret{Active: ver1}))

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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s2, should.Resemble(Secret{
				Active:  ver2,
				Passive: [][]byte{ver1},
			}))
		})

		t.Run("Stored secrets OK", func(t *ftt.Test) {
			gsm.createVersion("project", "secret", "v1")

			rotated := make(chan struct{})
			sm.AddRotationHandler(ctx, "sm://project/secret", func(_ context.Context, s Secret) {
				rotated <- struct{}{}
			})

			expectSleeping()
			s, err := sm.StoredSecret(ctx, "sm://project/secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s, should.Resemble(Secret{Active: []byte("v1")}))
			expectWoken(false)

			// Rotate the secret and make sure the change is picked up when expected.
			gsm.createVersion("project", "secret", "v2")
			expectFullSleep("2h16m23s")
			expectReloaded("sm://project/secret")

			s, err = sm.StoredSecret(ctx, "sm://project/secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s, should.Resemble(Secret{
				Active: []byte("v2"),
				Passive: [][]byte{
					[]byte("v1"),
				},
			}))

			<-rotated // doesn't hang
		})

		t.Run("Stored secrets exponential back-off", func(t *ftt.Test) {
			gsm.createVersion("project", "secret", "v1")

			expectSleeping()
			s, err := sm.StoredSecret(ctx, "sm://project/secret")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, s, should.Resemble(Secret{Active: []byte("v1")}))
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

		t.Run("Stored secrets priority queue", func(t *ftt.Test) {
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
	aliases  map[string]int    // full ref => version number
}

func (sm *secretManagerMock) createVersion(project, name, value string) string {
	sm.m.Lock()
	defer sm.m.Unlock()

	if sm.versions == nil {
		sm.versions = make(map[string]string, 1)
		sm.aliases = make(map[string]int, 1)
	}

	latestRef := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", project, name)

	nextVer := sm.aliases[latestRef] + 1
	versionRef := fmt.Sprintf("projects/%s/secrets/%s/versions/%d", project, name, nextVer)

	sm.versions[versionRef] = value
	sm.aliases[latestRef] = nextVer

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

func (sm *secretManagerMock) setAlias(alias, versionRef string) {
	sm.m.Lock()
	defer sm.m.Unlock()

	parts := strings.Split(versionRef, "/")

	ver, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil {
		panic(fmt.Sprintf("wrong version reference %s", versionRef))
	}

	parts[len(parts)-1] = alias
	aliasRef := strings.Join(parts, "/")

	sm.aliases[aliasRef] = int(ver)
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

	// Recognize aliases like "latest".
	versionRef := req.Name
	if ver := sm.aliases[versionRef]; ver != 0 {
		parts := strings.Split(versionRef, "/")
		parts[len(parts)-1] = fmt.Sprintf("%d", ver)
		versionRef = strings.Join(parts, "/")
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
