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

package coordinatorTest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/common/data/caching/cacheContext"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	configPB "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/config/common/cfgtypes"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/config"
	"go.chromium.org/luci/logdog/appengine/coordinator/endpoints"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex"
	"go.chromium.org/luci/logdog/common/storage/archive"
	"go.chromium.org/luci/logdog/common/storage/bigtable"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/settings"
	"go.chromium.org/luci/tumble"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
)

// AllAccessProject is the project name that can be used to get a full-access
// project (i.e. unauthenticated users have both R and W permissions).
const AllAccessProject = "proj-foo"

// mainServicePath is the path to the main service.
var mainServicePath string

func init() {
	mainServicePath = findParentDirectory("logdog", "appengine", "cmd", "coordinator", "default")
}

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
//
// If useRealIndex is true, this will attempt to load the 'index.yaml' file for
// logdog (but this is loaded from a relative path, so is only really good for
// the 'coordinator' package). Otherwise this will turn on datastore's automatic
// indexing functionality.
func Install(useRealIndex bool) (context.Context, *Environment) {
	e := Environment{
		Config:   make(map[string]memory.ConfigSet),
		GSClient: GSClient{},
		StorageCache: StorageCache{
			Base: &flex.StorageCache{},
		},
	}

	// Get our starting context. This installs, among other things, in-memory
	// gae, settings, and logger.
	c := e.Tumble.Context()
	c = caching.WithEmptyProcessCache(c)
	if *testGoLogger {
		c = logging.SetLevel(gologger.StdConfig.Use(c), logging.Debug)
	}

	// Create/install our BigTable memory instance.
	e.BigTable = bigtable.NewMemoryInstance(&e.StorageCache)

	if useRealIndex {
		// Load indexes from "index.yaml".
		indexDefs, err := ds.FindAndParseIndexYAML(mainServicePath)
		if err != nil {
			panic(fmt.Errorf("failed to load 'index.yaml': %s", err))
		}
		ds.GetTestable(c).AddIndexes(indexDefs...)
	} else {
		ds.GetTestable(c).AutoIndex(true)
	}

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
	addProjectConfig(AllAccessProject, "all:R", "all:W")
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
		ST: func(lst *coordinator.LogStreamState) (coordinator.SigningStorage, error) {
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

			base, err := archive.New(opts)
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
	c = coordinator.WithConfigProvider(c, &e.Services)
	c = endpoints.WithServices(c, &e.Services)
	c = flex.WithServices(c, &e.Services)

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

// findParentDirectory is used to traverse up from the current working directory
// to identify a target directory structure.
func findParentDirectory(paths ...string) string {
	base, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	// Build our basic directory scanning slice template, which consists of a
	// variable first element (root) and fixed set of remaining elements. We'll
	// switch out the first element during traversal.
	components := make([]string, 1, 1+len(paths))
	components[0] = base
	components = append(components, paths...)

	prev := ""
	for {
		candidate := filepath.Join(components...)
		if candidate == prev {
			panic(fmt.Errorf("could not find: %q", filepath.Join(paths...)))
		}

		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}

		prev = candidate
		components[0] = filepath.Dir(components[0])
	}
}
