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

package authdbimpl

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service"
	"go.chromium.org/luci/server/auth/service/protocol"
)

func TestConfigureAuthService(t *testing.T) {
	t.Parallel()

	ftt.Run("Initial config", t, func(t *ftt.Test) {
		srv := &fakeAuthService{LatestRev: 123}
		ctx := setAuthService(gaetesting.TestingContext(), srv)

		assert.Loosely(t, ConfigureAuthService(ctx, "http://base_url", "http://auth-service"), should.BeNil)
		assert.Loosely(t, srv.Calls, should.Resemble([]string{
			`EnsureSubscription "projects/app/subscriptions/dev-app-server-v1+auth-service" ""`,
		}))

		info, err := GetLatestSnapshotInfo(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, info, should.Resemble(&SnapshotInfo{
			AuthServiceURL: "http://auth-service",
			Rev:            123,
		}))

		// Coverage for GetAuthDBSnapshot.
		_, err = GetAuthDBSnapshot(ctx, "missing")
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))
		snap, err := GetAuthDBSnapshot(ctx, info.GetSnapshotID())
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, snap, should.Resemble(&protocol.AuthDB{
			OauthClientId:     "client-id-for-rev-123",
			OauthClientSecret: "secret",
		}))

		// Same config call again triggers resubsciption.
		srv.Calls = nil
		assert.Loosely(t, ConfigureAuthService(ctx, "http://base_url", "http://auth-service"), should.BeNil)
		assert.Loosely(t, srv.Calls, should.Resemble([]string{
			`EnsureSubscription "projects/app/subscriptions/dev-app-server-v1+auth-service" ""`,
		}))
	})

	ftt.Run("Switching cfg", t, func(t *ftt.Test) {
		srv := &fakeAuthService{LatestRev: 123}
		ctx := setAuthService(gaetesting.TestingContext(), srv)

		// Initial config.
		assert.Loosely(t, ConfigureAuthService(ctx, "http://base_url", "http://auth-service-1"), should.BeNil)
		// Change URL of the service.
		assert.Loosely(t, ConfigureAuthService(ctx, "http://base_url", "http://auth-service-2"), should.BeNil)

		info, err := GetLatestSnapshotInfo(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, info, should.Resemble(&SnapshotInfo{
			AuthServiceURL: "http://auth-service-2",
			Rev:            123,
		}))

		assert.Loosely(t, srv.Calls, should.Resemble([]string{
			`EnsureSubscription "projects/app/subscriptions/dev-app-server-v1+auth-service-1" ""`,
			`EnsureSubscription "projects/app/subscriptions/dev-app-server-v1+auth-service-2" ""`,
			`DeleteSubscription "projects/app/subscriptions/dev-app-server-v1+auth-service-1"`,
		}))
	})

	ftt.Run("Removing cfg", t, func(t *ftt.Test) {
		srv := &fakeAuthService{LatestRev: 123}
		ctx := setAuthService(gaetesting.TestingContext(), srv)

		// Initial config.
		assert.Loosely(t, ConfigureAuthService(ctx, "http://base_url", "http://auth-service-1"), should.BeNil)
		// Remove.
		assert.Loosely(t, ConfigureAuthService(ctx, "http://base_url", ""), should.BeNil)

		info, err := GetLatestSnapshotInfo(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, info, should.BeNil)

		assert.Loosely(t, srv.Calls, should.Resemble([]string{
			`EnsureSubscription "projects/app/subscriptions/dev-app-server-v1+auth-service-1" ""`,
			`DeleteSubscription "projects/app/subscriptions/dev-app-server-v1+auth-service-1"`,
		}))
	})
}

func TestSyncAuthDB(t *testing.T) {
	t.Parallel()

	ftt.Run("No new changes", t, func(t *ftt.Test) {
		srv := &fakeAuthService{LatestRev: 123}
		ctx := setAuthService(gaetesting.TestingContext(), srv)
		assert.Loosely(t, ConfigureAuthService(ctx, "http://base_url", "http://auth-service"), should.BeNil)

		info, err := syncAuthDB(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, info, should.Resemble(&SnapshotInfo{
			AuthServiceURL: "http://auth-service",
			Rev:            123,
		}))
	})

	ftt.Run("Have update", t, func(t *ftt.Test) {
		srv := &fakeAuthService{LatestRev: 123}
		ctx := setAuthService(gaetesting.TestingContext(), srv)
		assert.Loosely(t, ConfigureAuthService(ctx, "http://base_url", "http://auth-service"), should.BeNil)

		srv.LatestRev = 456

		info, err := syncAuthDB(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, info, should.Resemble(&SnapshotInfo{
			AuthServiceURL: "http://auth-service",
			Rev:            456,
		}))
	})
}

func TestSharding(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx := gaetesting.TestingContext()

		t.Run("Shard+unshard", func(t *ftt.Test) {
			shardIDs, err := shardAuthDB(ctx, "some-id", []byte("0123456789"), 3)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, shardIDs, should.Resemble([]string{
				"some-id:bf6aaaab7c143ca12ae448c69fb72bb4cf1b29154b9086a927a0a91ae334cdf7",
				"some-id:da70dfa4d9f95ac979f921e8e623358236313f334afcd06cddf8a5621cf6a1e9",
				"some-id:cebe3d9d614ba5c19f633566104315854a11353a333bf96f16b5afa0e90abdc4",
				"some-id:19581e27de7ced00ff1ce50b2047e7a567c76b1cbaebabe5ef03f7c3017bb5b7",
			}))

			t.Run("OK", func(t *ftt.Test) {
				blob, err := unshardAuthDB(ctx, shardIDs)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(blob), should.Equal("0123456789"))
			})

			t.Run("Missing one", func(t *ftt.Test) {
				_, err := unshardAuthDB(ctx, append(shardIDs, "missing"))
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
			})
		})

		t.Run("Store+load unsharded", func(t *ftt.Test) {
			assert.Loosely(t, storeDeflated(ctx, "some-id", []byte("0123456789"), time.Now(), 1000), should.BeNil)

			t.Run("OK", func(t *ftt.Test) {
				blob, _, err := fetchDeflated(ctx, "some-id")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(blob), should.Equal("0123456789"))
			})

			t.Run("Missing snapshot", func(t *ftt.Test) {
				_, code, err := fetchDeflated(ctx, "another-id")
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
				assert.Loosely(t, code, should.Equal("ERROR_NO_SNAPSHOT"))
			})
		})

		t.Run("Store+load sharded", func(t *ftt.Test) {
			assert.Loosely(t, storeDeflated(ctx, "some-id", []byte("0123456789"), time.Now(), 3), should.BeNil)

			t.Run("OK", func(t *ftt.Test) {
				blob, _, err := fetchDeflated(ctx, "some-id")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(blob), should.Equal("0123456789"))
			})

			t.Run("Missing snapshot", func(t *ftt.Test) {
				_, code, err := fetchDeflated(ctx, "another-id")
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
				assert.Loosely(t, code, should.Equal("ERROR_NO_SNAPSHOT"))
			})

			t.Run("Missing shard", func(t *ftt.Test) {
				// See the test above for the ID.
				datastore.Delete(ctx, datastore.KeyForObj(ctx, &SnapshotShard{
					ID: "some-id:bf6aaaab7c143ca12ae448c69fb72bb4cf1b29154b9086a927a0a91ae334cdf7",
				}))
				_, code, err := fetchDeflated(ctx, "some-id")
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
				assert.Loosely(t, code, should.Equal("ERROR_SHARDS_MISSING"))
			})
		})
	})
}

///

type fakeAuthService struct {
	LatestRev    int64
	Calls        []string
	Notification *service.Notification
}

func (f *fakeAuthService) EnsureSubscription(ctx context.Context, subscription, pushURL string) error {
	f.Calls = append(f.Calls, fmt.Sprintf("EnsureSubscription %q %q", subscription, pushURL))
	return nil
}

func (f *fakeAuthService) DeleteSubscription(ctx context.Context, subscription string) error {
	f.Calls = append(f.Calls, fmt.Sprintf("DeleteSubscription %q", subscription))
	return nil
}

func (f *fakeAuthService) PullPubSub(ctx context.Context, subscription string) (*service.Notification, error) {
	f.Calls = append(f.Calls, fmt.Sprintf("PullPubSub %q", subscription))
	return f.Notification, nil
}

func (f *fakeAuthService) ProcessPubSubPush(ctx context.Context, body []byte) (*service.Notification, error) {
	f.Calls = append(f.Calls, "ProcessPubSubPush")
	return f.Notification, nil
}

func (f *fakeAuthService) GetLatestSnapshotRevision(ctx context.Context) (int64, error) {
	return f.LatestRev, nil
}

func (f *fakeAuthService) GetSnapshot(ctx context.Context, rev int64) (*service.Snapshot, error) {
	if rev != f.LatestRev {
		return nil, fmt.Errorf("fakeAuthService: no snapshot for rev %d", rev)
	}
	return &service.Snapshot{
		AuthDB: &protocol.AuthDB{
			OauthClientId:     fmt.Sprintf("client-id-for-rev-%d", f.LatestRev),
			OauthClientSecret: "secret",
		},
		Rev: f.LatestRev,
	}, nil
}
