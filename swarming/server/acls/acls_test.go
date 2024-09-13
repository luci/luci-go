// Copyright 2023 The LUCI Authors.
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

package acls

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/cfg"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestServerLevel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	cfg := mockedConfig(&configpb.AuthSettings{
		AdminsGroup:          "admins",
		BotBootstrapGroup:    "bootstrap",
		PrivilegedUsersGroup: "privileged",
		ViewAllBotsGroup:     "view-all-bots",
		ViewAllTasksGroup:    "view-all-tasks",
	}, nil, nil)

	db := authtest.NewFakeDB(
		authtest.MockMembership("user:admin@example.com", "admins"),
		authtest.MockMembership("user:bootstrap@example.com", "bootstrap"),
		authtest.MockMembership("user:privileged@example.com", "privileged"),
		authtest.MockMembership("user:view-all-bots@example.com", "view-all-bots"),
		authtest.MockMembership("user:view-all-tasks@example.com", "view-all-tasks"),
	)

	permittedPerms := func(caller identity.Identity) []realms.Permission {
		chk := Checker{cfg: cfg, db: db, caller: caller}
		var permitted []realms.Permission
		for _, perm := range allPermissions() {
			res := chk.CheckServerPerm(ctx, perm)
			assert.Loosely(t, res.InternalError, should.BeFalse)
			if res.Permitted {
				permitted = append(permitted, perm)
			}
		}
		return permitted
	}

	ftt.Run("Unknown", t, func(t *ftt.Test) {
		assert.Loosely(t, permittedPerms("user:unknown@example.com"), should.BeEmpty)
	})

	ftt.Run("Admin", t, func(t *ftt.Test) {
		assertSame(t, permittedPerms("user:admin@example.com"), []realms.Permission{
			PermServersPeek,
			PermTasksGet,
			PermTasksCancel,
			PermPoolsListBots,
			PermPoolsListTasks,
			PermPoolsCreateBot,
			PermPoolsDeleteBot,
			PermPoolsTerminateBot,
			PermPoolsCancelTask,
		})
	})

	ftt.Run("Bootstrap", t, func(t *ftt.Test) {
		assertSame(t, permittedPerms("user:bootstrap@example.com"), []realms.Permission{
			PermServersPeek,
			PermPoolsCreateBot,
		})
	})

	ftt.Run("Privileged", t, func(t *ftt.Test) {
		assertSame(t, permittedPerms("user:privileged@example.com"), []realms.Permission{
			PermServersPeek,
			PermTasksGet,
			PermPoolsListBots,
			PermPoolsListTasks,
		})
	})

	ftt.Run("View all bots", t, func(t *ftt.Test) {
		assertSame(t, permittedPerms("user:view-all-bots@example.com"), []realms.Permission{
			PermServersPeek,
			PermPoolsListBots,
		})
	})

	ftt.Run("View all tasks", t, func(t *ftt.Test) {
		assertSame(t, permittedPerms("user:view-all-tasks@example.com"), []realms.Permission{
			PermServersPeek,
			PermTasksGet,
			PermPoolsListTasks,
		})
	})

	ftt.Run("Error message", t, func(t *ftt.Test) {
		chk := Checker{cfg: cfg, db: db, caller: "user:unknown@example.com"}
		res := chk.CheckServerPerm(ctx, PermTasksCancel)
		assert.Loosely(t, res.Permitted, should.BeFalse)
		err := res.ToGrpcErr()
		assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.PermissionDenied))
		assert.Loosely(t, err, should.ErrLike(`the caller "user:unknown@example.com" doesn't have server-level permission "swarming.tasks.cancel"`))

		err = res.ToTaggedError()
		assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		assert.Loosely(t, err, should.ErrLike(`the caller "user:unknown@example.com" doesn't have server-level permission "swarming.tasks.cancel"`))
	})
}

func TestPoolLevel(t *testing.T) {
	const (
		unknownID    identity.Identity = "user:unknown@example.com"
		privilegedID identity.Identity = "user:privileged@example.com"
		authorizedID identity.Identity = "user:authorized@example.com"
		anotherID    identity.Identity = "user:another@example.com"
	)

	t.Parallel()

	ctx := context.Background()

	allPools := []string{
		"visible-pool-1",
		"visible-pool-2",
		"hidden-pool-1",
		"hidden-pool-2",
		"deleted-pool-1", // has no realm association
		"deleted-pool-2", // has no realm association
	}

	cfg := mockedConfig(&configpb.AuthSettings{
		PrivilegedUsersGroup: "privileged",
	}, map[string]string{
		"visible-pool-1": "project:visible-realm",
		"visible-pool-2": "project:visible-realm",
		"hidden-pool-1":  "project:hidden-realm",
		"hidden-pool-2":  "project:hidden-realm",
	}, nil)

	db := authtest.NewFakeDB(
		authtest.MockMembership(privilegedID, "privileged"),
		authtest.MockPermission(authorizedID, "project:visible-realm", PermPoolsListBots),
		authtest.MockPermission(anotherID, "project:hidden-realm", PermPoolsListBots),
	)

	ftt.Run("CheckPoolPerm", t, func(t *ftt.Test) {
		// Note the implementation doesn't depend on exact permission being checked,
		// we'll check only PermPoolsListBots.
		poolsWithListBots := func(caller identity.Identity) []string {
			chk := Checker{cfg: cfg, db: db, caller: caller}
			var pools []string
			for _, pool := range allPools {
				res := chk.CheckPoolPerm(ctx, pool, PermPoolsListBots)
				assert.Loosely(t, res.InternalError, should.BeFalse)
				if res.Permitted {
					pools = append(pools, pool)
				}
			}
			return pools
		}

		t.Run("Unknown", func(t *ftt.Test) {
			assert.Loosely(t, poolsWithListBots(unknownID), should.BeEmpty)
		})

		t.Run("Privileged", func(t *ftt.Test) {
			assert.Loosely(t, poolsWithListBots(privilegedID), should.Resemble(allPools))
		})

		t.Run("Authorized", func(t *ftt.Test) {
			assert.Loosely(t, poolsWithListBots(authorizedID), should.Resemble([]string{
				"visible-pool-1",
				"visible-pool-2",
			}))
		})

		t.Run("Error message", func(t *ftt.Test) {
			chk := Checker{cfg: cfg, db: db, caller: unknownID}
			for _, pool := range allPools {
				res := chk.CheckPoolPerm(ctx, pool, PermPoolsListBots)
				assert.Loosely(t, res.Permitted, should.BeFalse)
				err := res.ToGrpcErr()
				assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(
					fmt.Sprintf(`the caller "user:unknown@example.com" doesn't have permission "swarming.pools.listBots"`+
						` in the pool %q or the pool doesn't exist`, pool)))
			}
		})
	})

	ftt.Run("FilterPoolsByPerm", t, func(t *ftt.Test) {
		poolsWithListBots := func(caller identity.Identity) []string {
			chk := Checker{cfg: cfg, db: db, caller: caller}
			pools, err := chk.FilterPoolsByPerm(ctx, allPools, PermPoolsListBots)
			assert.Loosely(t, err, should.BeNil)
			return pools
		}

		t.Run("Unknown", func(t *ftt.Test) {
			assert.Loosely(t, poolsWithListBots(unknownID), should.BeEmpty)
		})

		t.Run("Privileged", func(t *ftt.Test) {
			assert.Loosely(t, poolsWithListBots(privilegedID), should.Resemble(allPools))
		})

		t.Run("Authorized", func(t *ftt.Test) {
			assert.Loosely(t, poolsWithListBots(authorizedID), should.Resemble([]string{
				"visible-pool-1",
				"visible-pool-2",
			}))
		})
	})

	ftt.Run("CheckAllPoolsPerm", t, func(t *ftt.Test) {
		checkAll := func(caller identity.Identity, pools []string) bool {
			chk := Checker{cfg: cfg, db: db, caller: caller}
			res := chk.CheckAllPoolsPerm(ctx, pools, PermPoolsListBots)
			assert.Loosely(t, res.InternalError, should.BeFalse)
			return res.Permitted
		}

		t.Run("Unknown", func(t *ftt.Test) {
			assert.Loosely(t, checkAll(unknownID, []string{"visible-pool-1"}), should.BeFalse)
			assert.Loosely(t, checkAll(unknownID, []string{"visible-pool-1", "visible-pool-2"}), should.BeFalse)
			assert.Loosely(t, checkAll(unknownID, allPools), should.BeFalse)
		})

		t.Run("Privileged", func(t *ftt.Test) {
			assert.Loosely(t, checkAll(privilegedID, []string{"visible-pool-1"}), should.BeTrue)
			assert.Loosely(t, checkAll(privilegedID, []string{"visible-pool-1", "visible-pool-2"}), should.BeTrue)
			assert.Loosely(t, checkAll(privilegedID, []string{"hidden-pool-1"}), should.BeTrue)
			assert.Loosely(t, checkAll(privilegedID, []string{"hidden-pool-1", "hidden-pool-2"}), should.BeTrue)
			assert.Loosely(t, checkAll(privilegedID, allPools), should.BeTrue)
		})

		t.Run("Authorized", func(t *ftt.Test) {
			assert.Loosely(t, checkAll(authorizedID, []string{"visible-pool-1"}), should.BeTrue)
			assert.Loosely(t, checkAll(authorizedID, []string{"visible-pool-1", "visible-pool-2"}), should.BeTrue)
			assert.Loosely(t, checkAll(authorizedID, []string{"hidden-pool-1"}), should.BeFalse)
			assert.Loosely(t, checkAll(authorizedID, []string{"hidden-pool-1", "hidden-pool-2"}), should.BeFalse)
			assert.Loosely(t, checkAll(authorizedID, []string{"visible-pool-1", "visible-pool-2", "hidden-pool-1"}), should.BeFalse)
			assert.Loosely(t, checkAll(authorizedID, []string{"visible-pool-1", "visible-pool-2", "deleted-pool-1"}), should.BeFalse)
		})

		t.Run("Error message", func(t *ftt.Test) {
			chk := Checker{cfg: cfg, db: db, caller: authorizedID}
			res := chk.CheckAllPoolsPerm(ctx, allPools, PermPoolsListBots)
			assert.Loosely(t, res.InternalError, should.BeFalse)
			err := res.ToGrpcErr()
			assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike(`the caller "user:authorized@example.com" doesn't have permission "swarming.pools.listBots" in some of the requested pools`))
		})
	})

	ftt.Run("CheckAnyPoolsPerm", t, func(t *ftt.Test) {
		checkAny := func(caller identity.Identity, pools []string) bool {
			chk := Checker{cfg: cfg, db: db, caller: caller}
			res := chk.CheckAnyPoolsPerm(ctx, pools, PermPoolsListBots)
			assert.Loosely(t, res.InternalError, should.BeFalse)
			return res.Permitted
		}

		t.Run("Unknown", func(t *ftt.Test) {
			assert.Loosely(t, checkAny(unknownID, []string{"visible-pool-1"}), should.BeFalse)
			assert.Loosely(t, checkAny(unknownID, []string{"visible-pool-1", "visible-pool-2"}), should.BeFalse)
			assert.Loosely(t, checkAny(unknownID, allPools), should.BeFalse)
		})

		t.Run("Privileged", func(t *ftt.Test) {
			assert.Loosely(t, checkAny(privilegedID, []string{"visible-pool-1"}), should.BeTrue)
			assert.Loosely(t, checkAny(privilegedID, []string{"visible-pool-1", "visible-pool-2"}), should.BeTrue)
			assert.Loosely(t, checkAny(privilegedID, []string{"hidden-pool-1"}), should.BeTrue)
			assert.Loosely(t, checkAny(privilegedID, []string{"hidden-pool-1", "hidden-pool-2"}), should.BeTrue)
			assert.Loosely(t, checkAny(privilegedID, allPools), should.BeTrue)
		})

		t.Run("Authorized", func(t *ftt.Test) {
			assert.Loosely(t, checkAny(authorizedID, []string{"visible-pool-1"}), should.BeTrue)
			assert.Loosely(t, checkAny(authorizedID, []string{"visible-pool-1", "visible-pool-2"}), should.BeTrue)
			assert.Loosely(t, checkAny(authorizedID, []string{"hidden-pool-1"}), should.BeFalse)
			assert.Loosely(t, checkAny(authorizedID, []string{"hidden-pool-1", "hidden-pool-2"}), should.BeFalse)
			assert.Loosely(t, checkAny(authorizedID, []string{"deleted-pool-1"}), should.BeFalse)
			assert.Loosely(t, checkAny(authorizedID, []string{"deleted-pool-1", "deleted-pool-2"}), should.BeFalse)
			assert.Loosely(t, checkAny(authorizedID, []string{"hidden-pool-1", "visible-pool-1"}), should.BeTrue)
			assert.Loosely(t, checkAny(authorizedID, []string{"deleted-pool-1", "visible-pool-1"}), should.BeTrue)
		})

		t.Run("Error message", func(t *ftt.Test) {
			chk := Checker{cfg: cfg, db: db, caller: authorizedID}
			res := chk.CheckAnyPoolsPerm(ctx, []string{"hidden-pool-1", "deleted-pool-1"}, PermPoolsListBots)
			assert.Loosely(t, res.InternalError, should.BeFalse)
			err := res.ToGrpcErr()
			assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike(`the caller "user:authorized@example.com" doesn't have permission "swarming.pools.listBots" in any of the requested pools`))
		})
	})
}

func TestBotLevel(t *testing.T) {
	const (
		unknownID    identity.Identity = "user:unknown@example.com"
		privilegedID identity.Identity = "user:privileged@example.com"
		authorizedID identity.Identity = "user:authorized@example.com"
	)

	t.Parallel()

	ctx := context.Background()

	cfg := mockedConfig(&configpb.AuthSettings{
		PrivilegedUsersGroup: "privileged",
	}, map[string]string{
		"visible-pool": "project:visible-realm",
		"hidden-pool":  "project:hidden-realm",
	}, map[string][]string{
		"visible-bot": {"visible-pool", "hidden-pool"},
		"hidden-bot":  {"hidden-pool"},
	})

	db := authtest.NewFakeDB(
		authtest.MockMembership(privilegedID, "privileged"),
		authtest.MockPermission(authorizedID, "project:visible-realm", PermPoolsListBots),
	)

	checkBotVisible := func(caller identity.Identity, botID string) bool {
		chk := Checker{cfg: cfg, db: db, caller: caller}
		res := chk.CheckBotPerm(ctx, botID, PermPoolsListBots)
		assert.Loosely(t, res.InternalError, should.BeFalse)
		return res.Permitted
	}

	ftt.Run("Unknown", t, func(t *ftt.Test) {
		assert.Loosely(t, checkBotVisible(unknownID, "visible-bot"), should.BeFalse)
		assert.Loosely(t, checkBotVisible(unknownID, "hidden-bot"), should.BeFalse)
		assert.Loosely(t, checkBotVisible(unknownID, "unknown-bot"), should.BeFalse)
	})

	ftt.Run("Privileged", t, func(t *ftt.Test) {
		assert.Loosely(t, checkBotVisible(privilegedID, "visible-bot"), should.BeTrue)
		assert.Loosely(t, checkBotVisible(privilegedID, "hidden-bot"), should.BeTrue)
		assert.Loosely(t, checkBotVisible(privilegedID, "unknown-bot"), should.BeTrue)
	})

	ftt.Run("Authorized", t, func(t *ftt.Test) {
		assert.Loosely(t, checkBotVisible(authorizedID, "visible-bot"), should.BeTrue)
		assert.Loosely(t, checkBotVisible(authorizedID, "hidden-bot"), should.BeFalse)
		assert.Loosely(t, checkBotVisible(authorizedID, "unknown-bot"), should.BeFalse)
	})

	ftt.Run("Error message", t, func(t *ftt.Test) {
		chk := Checker{cfg: cfg, db: db, caller: authorizedID}
		res := chk.CheckBotPerm(ctx, "hidden-bot", PermPoolsListBots)
		assert.Loosely(t, res.InternalError, should.BeFalse)
		err := res.ToGrpcErr()
		assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.PermissionDenied))
		assert.Loosely(t, err, should.ErrLike(`the caller "user:authorized@example.com" doesn't have permission `+
			`"swarming.pools.listBots" in the pool that contains bot "hidden-bot" or this bot doesn't exist`))
	})
}

func TestTaskLevel(t *testing.T) {
	const (
		unknownID    identity.Identity = "user:unknown@example.com"
		adminID      identity.Identity = "user:admin@example.com"
		authorizedID identity.Identity = "user:authorized@example.com"
	)

	t.Parallel()

	ctx := context.Background()

	cfg := mockedConfig(&configpb.AuthSettings{
		AdminsGroup: "admins",
	}, map[string]string{
		"visible-pool": "project:visible-pool-realm",
		"hidden-pool":  "project:hidden-pool-realm",
	}, map[string][]string{
		"visible-bot": {"visible-pool", "hidden-pool"},
		"hidden-bot":  {"hidden-pool"},
	})

	db := authtest.NewFakeDB(
		authtest.MockMembership(adminID, "admins"),
		authtest.MockPermission(authorizedID, "project:visible-task-realm", PermTasksCancel),
		authtest.MockPermission(authorizedID, "project:visible-pool-realm", PermPoolsCancelTask),
	)

	checkCanCancel := func(caller identity.Identity, info TaskAuthInfo) bool {
		info.TaskID = "65aba3a3e6b99310"
		chk := Checker{cfg: cfg, db: db, caller: caller}
		res := chk.CheckTaskPerm(ctx, &mockedTask{info: info}, PermTasksCancel)
		assert.Loosely(t, res.InternalError, should.BeFalse)
		return res.Permitted
	}

	ftt.Run("Unknown", t, func(t *ftt.Test) {
		assert.Loosely(t, checkCanCancel(unknownID, TaskAuthInfo{
			Realm:     "project:visible-task-realm",
			Pool:      "visible-pool",
			Submitter: authorizedID,
		}), should.BeFalse)
	})

	ftt.Run("Submitter", t, func(t *ftt.Test) {
		assert.Loosely(t, checkCanCancel(unknownID, TaskAuthInfo{
			Realm:     "project:doesnt-matter",
			Pool:      "doesnt-matter",
			Submitter: unknownID,
		}), should.BeTrue)
	})

	ftt.Run("Admin", t, func(t *ftt.Test) {
		assert.Loosely(t, checkCanCancel(adminID, TaskAuthInfo{
			Realm:     "project:doesnt-matter",
			Pool:      "doesnt-matter",
			Submitter: unknownID,
		}), should.BeTrue)
	})

	ftt.Run("Via task realm", t, func(t *ftt.Test) {
		assert.Loosely(t, checkCanCancel(authorizedID, TaskAuthInfo{
			Realm:     "project:visible-task-realm",
			Pool:      "doesnt-matter",
			Submitter: unknownID,
		}), should.BeTrue)

		assert.Loosely(t, checkCanCancel(authorizedID, TaskAuthInfo{
			Realm:     "project:hidden-task-realm",
			Pool:      "doesnt-matter",
			Submitter: unknownID,
		}), should.BeFalse)
	})

	ftt.Run("Via pool realm", t, func(t *ftt.Test) {
		assert.Loosely(t, checkCanCancel(authorizedID, TaskAuthInfo{
			Realm:     "project:doesnt-matter",
			Pool:      "visible-pool",
			Submitter: unknownID,
		}), should.BeTrue)

		assert.Loosely(t, checkCanCancel(authorizedID, TaskAuthInfo{
			Realm:     "project:doesnt-matter",
			Pool:      "hidden-pool",
			Submitter: unknownID,
		}), should.BeFalse)
	})

	ftt.Run("Via bot realm", t, func(t *ftt.Test) {
		assert.Loosely(t, checkCanCancel(authorizedID, TaskAuthInfo{
			Realm:     "project:doesnt-matter",
			BotID:     "visible-bot",
			Submitter: unknownID,
		}), should.BeTrue)

		assert.Loosely(t, checkCanCancel(authorizedID, TaskAuthInfo{
			Realm:     "project:doesnt-matter",
			BotID:     "hidden-bot",
			Submitter: unknownID,
		}), should.BeFalse)
	})

	ftt.Run("Error message", t, func(t *ftt.Test) {
		chk := Checker{cfg: cfg, db: db, caller: authorizedID}
		res := chk.CheckTaskPerm(ctx, &mockedTask{
			info: TaskAuthInfo{
				TaskID:    "65aba3a3e6b99310",
				Realm:     "project:doesnt-matter",
				BotID:     "hidden-bot",
				Submitter: unknownID,
			},
		}, PermTasksCancel)
		assert.Loosely(t, res.InternalError, should.BeFalse)
		err := res.ToGrpcErr()
		assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.PermissionDenied))
		assert.Loosely(t, err, should.ErrLike(`the caller "user:authorized@example.com" doesn't have `+
			`permission "swarming.tasks.cancel" for the task "65aba3a3e6b99310"`))
	})

	ftt.Run("Error from TaskAuthInfo", t, func(t *ftt.Test) {
		chk := Checker{cfg: cfg, db: db, caller: authorizedID}
		res := chk.CheckTaskPerm(ctx, &mockedTask{err: errors.New("BOOM")}, PermTasksCancel)
		assert.Loosely(t, res.InternalError, should.BeTrue)
		assert.Loosely(t, res.ToGrpcErr(), convey.Adapt(ShouldHaveGRPCStatus)(codes.Internal))
		assert.Loosely(t, transient.Tag.In(res.ToTaggedError()), should.BeTrue)
	})
}

////////////////////////////////////////////////////////////////////////////////

// allPermissions returns all registered Swarming permissions.
func allPermissions() []realms.Permission {
	var perms []realms.Permission
	for perm := range realms.RegisteredPermissions() {
		if strings.HasPrefix(perm.String(), "swarming.") {
			perms = append(perms, perm)
		}
	}
	sort.Slice(perms, func(i, j int) bool { return perms[i].String() < perms[j].String() })
	return perms
}

// assertSame fails if permissions sets have differences.
func assertSame(t testing.TB, got, want []realms.Permission) {
	t.Helper()

	asSet := func(x []realms.Permission) []string {
		s := stringset.New(len(x))
		for _, p := range x {
			s.Add(p.String())
		}
		return s.ToSortedSlice()
	}
	assert.Loosely(t, asSet(got), should.Resemble(asSet(want)), truth.LineContext())
}

// mockedConfig prepares a queryable config.
func mockedConfig(settings *configpb.AuthSettings, pools map[string]string, bots map[string][]string) *cfg.Config {
	// Note this logic is the same as in rpcs.MockConfigs, but we can't use it
	// directly due to import cycles. It is a relatively small chunk of code, it's
	// not worth extracting into a separate package. So just repeat it.
	//
	// It essentially uses the real config loading implementation, just on top of
	// fake temporary datastore.

	// Prepare minimal pools.cfg.
	var poolpb []*configpb.Pool
	for pool, realm := range pools {
		poolpb = append(poolpb, &configpb.Pool{
			Name:  []string{pool},
			Realm: realm,
		})
	}
	sort.Slice(poolpb, func(i, j int) bool { return poolpb[i].Name[0] < poolpb[j].Name[0] })

	// Prepare minimal bots.cfg.
	var botpb []*configpb.BotGroup
	for botID, pools := range bots {
		var dims []string
		for _, pool := range pools {
			dims = append(dims, "pool:"+pool)
		}
		botpb = append(botpb, &configpb.BotGroup{
			BotId:      []string{botID},
			Dimensions: dims,
			Auth: []*configpb.BotAuth{ // required field
				{
					RequireLuciMachineToken: true,
				},
			},
		})
	}
	sort.Slice(botpb, func(i, j int) bool { return botpb[i].BotId[0] < botpb[j].BotId[0] })

	// Convert configs to raw proto text files.
	files := make(cfgmem.Files)
	putPb := func(path string, msg proto.Message) {
		if msg != nil {
			blob, err := prototext.Marshal(msg)
			if err != nil {
				panic(err)
			}
			files[path] = string(blob)
		}
	}
	putPb("settings.cfg", &configpb.SettingsCfg{Auth: settings})
	putPb("pools.cfg", &configpb.PoolsCfg{Pool: poolpb})
	putPb("bots.cfg", &configpb.BotsCfg{
		TrustedDimensions: []string{"pool"},
		BotGroup:          botpb,
	})

	// Put new configs into a temporary fake datastore.
	ctx := memory.Use(context.Background())
	err := cfg.UpdateConfigs(cfgclient.Use(ctx, cfgmem.New(map[config.Set]cfgmem.Files{
		"services/${appid}": files,
	})), nil, nil)
	if err != nil {
		panic(err)
	}

	// Load them back in a queriable form.
	p, err := cfg.NewProvider(ctx)
	if err != nil {
		panic(err)
	}
	return p.Cached(ctx)
}

type mockedTask struct {
	info TaskAuthInfo
	err  error
}

func (m *mockedTask) TaskAuthInfo(ctx context.Context) (*TaskAuthInfo, error) {
	return &m.info, m.err
}
