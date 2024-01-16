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
	"fmt"
	"sort"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/cfg"

	. "github.com/smartystreets/goconvey/convey"
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
			So(res.InternalError, ShouldBeFalse)
			if res.Permitted {
				permitted = append(permitted, perm)
			}
		}
		return permitted
	}

	Convey("Unknown", t, func() {
		So(permittedPerms("user:unknown@example.com"), ShouldBeEmpty)
	})

	Convey("Admin", t, func() {
		assertSame(permittedPerms("user:admin@example.com"), []realms.Permission{
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

	Convey("Bootstrap", t, func() {
		assertSame(permittedPerms("user:bootstrap@example.com"), []realms.Permission{
			PermPoolsCreateBot,
		})
	})

	Convey("Privileged", t, func() {
		assertSame(permittedPerms("user:privileged@example.com"), []realms.Permission{
			PermTasksGet,
			PermPoolsListBots,
			PermPoolsListTasks,
		})
	})

	Convey("View all bots", t, func() {
		assertSame(permittedPerms("user:view-all-bots@example.com"), []realms.Permission{
			PermPoolsListBots,
		})
	})

	Convey("View all tasks", t, func() {
		assertSame(permittedPerms("user:view-all-tasks@example.com"), []realms.Permission{
			PermTasksGet,
			PermPoolsListTasks,
		})
	})

	Convey("Error message", t, func() {
		chk := Checker{cfg: cfg, db: db, caller: "user:unknown@example.com"}
		res := chk.CheckServerPerm(ctx, PermTasksCancel)
		So(res.Permitted, ShouldBeFalse)
		err := res.ToGrpcErr()
		So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
		So(err, ShouldErrLike, `the caller "user:unknown@example.com" doesn't have server-level permission "swarming.tasks.cancel"`)
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

	Convey("CheckPoolPerm", t, func() {
		// Note the implementation doesn't depend on exact permission being checked,
		// we'll check only PermPoolsListBots.
		poolsWithListBots := func(caller identity.Identity) []string {
			chk := Checker{cfg: cfg, db: db, caller: caller}
			var pools []string
			for _, pool := range allPools {
				res := chk.CheckPoolPerm(ctx, pool, PermPoolsListBots)
				So(res.InternalError, ShouldBeFalse)
				if res.Permitted {
					pools = append(pools, pool)
				}
			}
			return pools
		}

		Convey("Unknown", func() {
			So(poolsWithListBots(unknownID), ShouldBeEmpty)
		})

		Convey("Privileged", func() {
			So(poolsWithListBots(privilegedID), ShouldResemble, allPools)
		})

		Convey("Authorized", func() {
			So(poolsWithListBots(authorizedID), ShouldResemble, []string{
				"visible-pool-1",
				"visible-pool-2",
			})
		})

		Convey("Error message", func() {
			chk := Checker{cfg: cfg, db: db, caller: unknownID}
			for _, pool := range allPools {
				res := chk.CheckPoolPerm(ctx, pool, PermPoolsListBots)
				So(res.Permitted, ShouldBeFalse)
				err := res.ToGrpcErr()
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
				So(err, ShouldErrLike,
					fmt.Sprintf(`the caller "user:unknown@example.com" doesn't have permission "swarming.pools.listBots"`+
						` in the pool %q or the pool doesn't exist`, pool))
			}
		})
	})

	Convey("FilterPoolsByPerm", t, func() {
		poolsWithListBots := func(caller identity.Identity) []string {
			chk := Checker{cfg: cfg, db: db, caller: caller}
			pools, err := chk.FilterPoolsByPerm(ctx, allPools, PermPoolsListBots)
			So(err, ShouldBeNil)
			return pools
		}

		Convey("Unknown", func() {
			So(poolsWithListBots(unknownID), ShouldBeEmpty)
		})

		Convey("Privileged", func() {
			So(poolsWithListBots(privilegedID), ShouldResemble, allPools)
		})

		Convey("Authorized", func() {
			So(poolsWithListBots(authorizedID), ShouldResemble, []string{
				"visible-pool-1",
				"visible-pool-2",
			})
		})
	})

	Convey("CheckAllPoolsPerm", t, func() {
		checkAll := func(caller identity.Identity, pools []string) bool {
			chk := Checker{cfg: cfg, db: db, caller: caller}
			res := chk.CheckAllPoolsPerm(ctx, pools, PermPoolsListBots)
			So(res.InternalError, ShouldBeFalse)
			return res.Permitted
		}

		Convey("Unknown", func() {
			So(checkAll(unknownID, []string{"visible-pool-1"}), ShouldBeFalse)
			So(checkAll(unknownID, []string{"visible-pool-1", "visible-pool-2"}), ShouldBeFalse)
			So(checkAll(unknownID, allPools), ShouldBeFalse)
		})

		Convey("Privileged", func() {
			So(checkAll(privilegedID, []string{"visible-pool-1"}), ShouldBeTrue)
			So(checkAll(privilegedID, []string{"visible-pool-1", "visible-pool-2"}), ShouldBeTrue)
			So(checkAll(privilegedID, []string{"hidden-pool-1"}), ShouldBeTrue)
			So(checkAll(privilegedID, []string{"hidden-pool-1", "hidden-pool-2"}), ShouldBeTrue)
			So(checkAll(privilegedID, allPools), ShouldBeTrue)
		})

		Convey("Authorized", func() {
			So(checkAll(authorizedID, []string{"visible-pool-1"}), ShouldBeTrue)
			So(checkAll(authorizedID, []string{"visible-pool-1", "visible-pool-2"}), ShouldBeTrue)
			So(checkAll(authorizedID, []string{"hidden-pool-1"}), ShouldBeFalse)
			So(checkAll(authorizedID, []string{"hidden-pool-1", "hidden-pool-2"}), ShouldBeFalse)
			So(checkAll(authorizedID, []string{"visible-pool-1", "visible-pool-2", "hidden-pool-1"}), ShouldBeFalse)
			So(checkAll(authorizedID, []string{"visible-pool-1", "visible-pool-2", "deleted-pool-1"}), ShouldBeFalse)
		})

		Convey("Error message", func() {
			chk := Checker{cfg: cfg, db: db, caller: authorizedID}
			res := chk.CheckAllPoolsPerm(ctx, allPools, PermPoolsListBots)
			So(res.InternalError, ShouldBeFalse)
			err := res.ToGrpcErr()
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, `the caller "user:authorized@example.com" doesn't have permission "swarming.pools.listBots" in some of the requested pools`)
		})
	})

	Convey("CheckAnyPoolsPerm", t, func() {
		checkAny := func(caller identity.Identity, pools []string) bool {
			chk := Checker{cfg: cfg, db: db, caller: caller}
			res := chk.CheckAnyPoolsPerm(ctx, pools, PermPoolsListBots)
			So(res.InternalError, ShouldBeFalse)
			return res.Permitted
		}

		Convey("Unknown", func() {
			So(checkAny(unknownID, []string{"visible-pool-1"}), ShouldBeFalse)
			So(checkAny(unknownID, []string{"visible-pool-1", "visible-pool-2"}), ShouldBeFalse)
			So(checkAny(unknownID, allPools), ShouldBeFalse)
		})

		Convey("Privileged", func() {
			So(checkAny(privilegedID, []string{"visible-pool-1"}), ShouldBeTrue)
			So(checkAny(privilegedID, []string{"visible-pool-1", "visible-pool-2"}), ShouldBeTrue)
			So(checkAny(privilegedID, []string{"hidden-pool-1"}), ShouldBeTrue)
			So(checkAny(privilegedID, []string{"hidden-pool-1", "hidden-pool-2"}), ShouldBeTrue)
			So(checkAny(privilegedID, allPools), ShouldBeTrue)
		})

		Convey("Authorized", func() {
			So(checkAny(authorizedID, []string{"visible-pool-1"}), ShouldBeTrue)
			So(checkAny(authorizedID, []string{"visible-pool-1", "visible-pool-2"}), ShouldBeTrue)
			So(checkAny(authorizedID, []string{"hidden-pool-1"}), ShouldBeFalse)
			So(checkAny(authorizedID, []string{"hidden-pool-1", "hidden-pool-2"}), ShouldBeFalse)
			So(checkAny(authorizedID, []string{"deleted-pool-1"}), ShouldBeFalse)
			So(checkAny(authorizedID, []string{"deleted-pool-1", "deleted-pool-2"}), ShouldBeFalse)
			So(checkAny(authorizedID, []string{"hidden-pool-1", "visible-pool-1"}), ShouldBeTrue)
			So(checkAny(authorizedID, []string{"deleted-pool-1", "visible-pool-1"}), ShouldBeTrue)
		})

		Convey("Error message", func() {
			chk := Checker{cfg: cfg, db: db, caller: authorizedID}
			res := chk.CheckAnyPoolsPerm(ctx, []string{"hidden-pool-1", "deleted-pool-1"}, PermPoolsListBots)
			So(res.InternalError, ShouldBeFalse)
			err := res.ToGrpcErr()
			So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, `the caller "user:authorized@example.com" doesn't have permission "swarming.pools.listBots" in any of the requested pools`)
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
		So(res.InternalError, ShouldBeFalse)
		return res.Permitted
	}

	Convey("Unknown", t, func() {
		So(checkBotVisible(unknownID, "visible-bot"), ShouldBeFalse)
		So(checkBotVisible(unknownID, "hidden-bot"), ShouldBeFalse)
		So(checkBotVisible(unknownID, "unknown-bot"), ShouldBeFalse)
	})

	Convey("Privileged", t, func() {
		So(checkBotVisible(privilegedID, "visible-bot"), ShouldBeTrue)
		So(checkBotVisible(privilegedID, "hidden-bot"), ShouldBeTrue)
		So(checkBotVisible(privilegedID, "unknown-bot"), ShouldBeTrue)
	})

	Convey("Authorized", t, func() {
		So(checkBotVisible(authorizedID, "visible-bot"), ShouldBeTrue)
		So(checkBotVisible(authorizedID, "hidden-bot"), ShouldBeFalse)
		So(checkBotVisible(authorizedID, "unknown-bot"), ShouldBeFalse)
	})

	Convey("Error message", t, func() {
		chk := Checker{cfg: cfg, db: db, caller: authorizedID}
		res := chk.CheckBotPerm(ctx, "hidden-bot", PermPoolsListBots)
		So(res.InternalError, ShouldBeFalse)
		err := res.ToGrpcErr()
		So(err, ShouldHaveGRPCStatus, codes.PermissionDenied)
		So(err, ShouldErrLike, `the caller "user:authorized@example.com" doesn't have permission `+
			`"swarming.pools.listBots" in the pool that contains bot "hidden-bot" or this bot doesn't exist`)
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
func assertSame(got, want []realms.Permission) {
	asSet := func(x []realms.Permission) []string {
		s := stringset.New(len(x))
		for _, p := range x {
			s.Add(p.String())
		}
		return s.ToSortedSlice()
	}
	So(asSet(got), ShouldResemble, asSet(want))
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
	})))
	if err != nil {
		panic(err)
	}

	// Load them back in a queriable form.
	p, err := cfg.NewProvider(ctx)
	if err != nil {
		panic(err)
	}
	return p.Config(ctx)
}
