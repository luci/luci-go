// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package coordinatorTest

import (
	"fmt"
	"strings"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/data/caching/cacheContext"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	configPB "github.com/luci/luci-go/common/proto/config"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/logdog/api/config/svcconfig"
	"github.com/luci/luci-go/logdog/appengine/coordinator"
	"github.com/luci/luci-go/logdog/appengine/coordinator/config"
	"github.com/luci/luci-go/logdog/common/storage/archive"
	"github.com/luci/luci-go/logdog/common/storage/bigtable"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/testconfig"
	"github.com/luci/luci-go/luci_config/server/cfgclient/textproto"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/settings"
	"github.com/luci/luci-go/tumble"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

// Environment contains all of the testing facilities that are installed into
// the Context.
type Environment struct {
	// Tumble is the Tumble testing instance.
	Tumble tumble.Testing

	// Clock is the installed test clock instance.
	Clock testclock.TestClock

	// AuthState is the fake authentication state.
	AuthState authtest.FakeState

	// Config is the luci-config configuration map that is installed.
	Config map[string]memory.ConfigSet

	// Services is the set of installed Coordinator services.
	Services Services

	// BigTable in-memory testing instance.
	BigTable bigtable.Testing
	// GSClient is the test GSClient instance installed (by default) into
	// Services.
	GSClient GSClient

	// ArchivalPublisher is the test ArchivalPublisher instance installed (by
	// default) into Services.
	ArchivalPublisher ArchivalPublisher

	// StorageCache is the default storage cache instance.
	StorageCache StorageCache
}

// LogIn installs an testing identity into the testing auth state.
func (e *Environment) LogIn() {
	id, err := identity.MakeIdentity("user:testing@example.com")
	if err != nil {
		panic(err)
	}
	e.AuthState.Identity = id
}

// JoinGroup adds the named group the to the list of groups for the current
// identity.
func (e *Environment) JoinGroup(g string) {
	e.AuthState.IdentityGroups = append(e.AuthState.IdentityGroups, g)
}

// LeaveAllGroups clears all auth groups that the user is currently a member of.
func (e *Environment) LeaveAllGroups() {
	e.AuthState.IdentityGroups = nil
}

// ClearCoordinatorConfig removes the Coordinator configuration entry,
// simulating a missing config.
func (e *Environment) ClearCoordinatorConfig(c context.Context) {
	configSet, _ := config.ServiceConfigPath(c)
	delete(e.Config, string(configSet))
}

// ModServiceConfig loads the current service configuration, invokes the
// callback with its contents, and writes the result back to config.
func (e *Environment) ModServiceConfig(c context.Context, fn func(*svcconfig.Config)) {
	configSet, configPath := config.ServiceConfigPath(c)

	var cfg svcconfig.Config
	e.modTextProtobuf(c, configSet, configPath, &cfg, func() {
		fn(&cfg)
	})
}

// ModProjectConfig loads the current configuration for the named project,
// invokes the callback with its contents, and writes the result back to config.
func (e *Environment) ModProjectConfig(c context.Context, proj cfgtypes.ProjectName, fn func(*svcconfig.ProjectConfig)) {
	configSet, configPath := cfgtypes.ProjectConfigSet(proj), config.ProjectConfigPath(c)

	var pcfg svcconfig.ProjectConfig
	e.modTextProtobuf(c, configSet, configPath, &pcfg, func() {
		fn(&pcfg)
	})
}

// IterateTumbleAll iterates all Tumble instances across all namespaces.
func (e *Environment) IterateTumbleAll(c context.Context) { e.Tumble.IterateAll(c) }

func (e *Environment) modTextProtobuf(c context.Context, configSet cfgtypes.ConfigSet, path string,
	msg proto.Message, fn func()) {

	switch err := cfgclient.Get(c, cfgclient.AsService, configSet, path, textproto.Message(msg), nil); err {
	case nil, cfgclient.ErrNoConfig:
		break
	default:
		panic(err)
	}

	fn()
	e.addConfigEntry(configSet, path, proto.MarshalTextString(msg))
}

func (e *Environment) addConfigEntry(configSet cfgtypes.ConfigSet, path, content string) {
	cset := e.Config[string(configSet)]
	if cset == nil {
		cset = make(map[string]string)
		e.Config[string(configSet)] = cset
	}
	cset[path] = content
}

// Install creates a testing Context and installs common test facilities into
// it, returning the Environment to which they're bound.
func Install() (context.Context, *Environment) {
	e := Environment{
		Config:   make(map[string]memory.ConfigSet),
		GSClient: GSClient{},
		StorageCache: StorageCache{
			Base: &coordinator.StorageCache{},
		},
	}

	// Get our starting context. This installs, among other things, in-memory
	// gae, settings, and logger.
	c := e.Tumble.Context()
	if *testGoLogger {
		c = logging.SetLevel(gologger.StdConfig.Use(c), logging.Debug)
	}

	// Create/install our BigTable memory instance.
	e.BigTable = bigtable.NewMemoryInstance(c, bigtable.Options{
		Cache: &e.StorageCache,
	})

	// Add indexes. These should match the indexes defined in the application's
	// "index.yaml".
	indexDefs := [][]string{
		{"Prefix", "-Created"},
		{"Name", "-Created"},
		{"State", "-Created"},
		{"Purged", "-Created"},
		{"ProtoVersion", "-Created"},
		{"ContentType", "-Created"},
		{"StreamType", "-Created"},
		{"Timestamp", "-Created"},
		{"_C", "-Created"},
		{"_Tags", "-Created"},
		{"_Terminated", "-Created"},
		{"_Archived", "-Created"},
	}
	indexes := make([]*ds.IndexDefinition, len(indexDefs))
	for i, id := range indexDefs {
		cols := make([]ds.IndexColumn, len(id))
		for j, ic := range id {
			var err error
			cols[j], err = ds.ParseIndexColumn(ic)
			if err != nil {
				panic(fmt.Errorf("failed to parse index %q: %s", ic, err))
			}
		}
		indexes[i] = &ds.IndexDefinition{Kind: "LogStream", SortBy: cols}
	}
	ds.GetTestable(c).AddIndexes(indexes...)

	// Setup clock.
	e.Clock = clock.Get(c).(testclock.TestClock)

	// Install GAE config service settings.
	c = settings.Use(c, settings.New(&settings.MemoryStorage{}))

	// Setup luci-config configuration.
	c = testconfig.WithCommonClient(c, memory.New(e.Config))

	// luci-config: Projects.
	projectName := info.AppID(c)
	addProjectConfig := func(proj cfgtypes.ProjectName, access ...string) {
		projectAccesses := make([]string, len(access))

		// Build our service config. Also builds "projectAccesses".
		e.ModProjectConfig(c, proj, func(pcfg *svcconfig.ProjectConfig) {
			for i, a := range access {
				parts := strings.SplitN(a, ":", 2)
				group, field := parts[0], &pcfg.ReaderAuthGroups
				if len(parts) == 2 {
					switch parts[1] {
					case "R":
						break
					case "W":
						field = &pcfg.WriterAuthGroups
					default:
						panic(a)
					}
				}
				*field = append(*field, group)
				projectAccesses[i] = fmt.Sprintf("group:%s", group)
			}
		})

		var pcfg configPB.ProjectCfg
		e.modTextProtobuf(c, cfgtypes.ProjectConfigSet(proj), cfgclient.ProjectConfigPath, &pcfg, func() {
			pcfg = configPB.ProjectCfg{
				Name:   proto.String(string(proj)),
				Access: projectAccesses,
			}
		})
	}
	addProjectConfig("proj-foo", "all:R", "all:W")
	addProjectConfig("proj-bar", "all:R", "auth:W")
	addProjectConfig("proj-exclusive", "auth:R", "auth:W")

	// Add a project without a LogDog project config.
	e.addConfigEntry("projects/proj-unconfigured", "not-logdog.cfg", "junk")

	configSet, configPath := cfgtypes.ProjectConfigSet("proj-malformed"), config.ProjectConfigPath(c)
	e.addConfigEntry(configSet, configPath, "!!! not a text protobuf !!!")

	// luci-config: Coordinator Defaults
	e.ModServiceConfig(c, func(cfg *svcconfig.Config) {
		cfg.Transport = &svcconfig.Transport{
			Type: &svcconfig.Transport_Pubsub{
				Pubsub: &svcconfig.Transport_PubSub{
					Project: projectName,
					Topic:   "test-topic",
				},
			},
		}
		cfg.Coordinator = &svcconfig.Coordinator{
			AdminAuthGroup:   "admin",
			ServiceAuthGroup: "services",
			PrefixExpiration: google.NewDuration(24 * time.Hour),
		}
	})

	// Setup Tumble. This also adds the two Tumble indexes to datastore.
	e.Tumble.EnableDelayedMutations(c)

	tcfg := e.Tumble.GetConfig(c)
	tcfg.TemporalRoundFactor = 0 // Makes test timing easier to understand.
	tcfg.TemporalMinDelay = 0    // Makes test timing easier to understand.
	e.Tumble.UpdateSettings(c, tcfg)

	// Install authentication state.
	c = auth.WithState(c, &e.AuthState)

	// Setup authentication state.
	e.LeaveAllGroups()
	e.JoinGroup("all")

	// Setup our default Coordinator services.
	e.Services = Services{
		ST: func(lst *coordinator.LogStreamState) (coordinator.Storage, error) {
			// If we're not archived, return our BigTable storage instance.
			if !lst.ArchivalState().Archived() {
				return &BigTableStorage{
					Testing: e.BigTable,
				}, nil
			}

			opts := archive.Options{
				Index:  gs.Path(lst.ArchiveIndexURL),
				Stream: gs.Path(lst.ArchiveStreamURL),
				Client: &e.GSClient,
				Cache:  &e.StorageCache,
			}

			base, err := archive.New(c, opts)
			if err != nil {
				return nil, err
			}
			return &ArchivalStorage{
				Storage: base,
				Opts:    opts,
			}, nil
		},
		AP: func() (coordinator.ArchivalPublisher, error) {
			return &e.ArchivalPublisher, nil
		},
	}
	c = coordinator.WithServices(c, &e.Services)

	return cacheContext.Wrap(c), &e
}

// WithProjectNamespace runs f in proj's namespace, bypassing authentication
// checks.
func WithProjectNamespace(c context.Context, proj cfgtypes.ProjectName, f func(context.Context)) {
	if err := coordinator.WithProjectNamespace(&c, proj, coordinator.NamespaceAccessAllTesting); err != nil {
		panic(err)
	}
	f(c)
}
