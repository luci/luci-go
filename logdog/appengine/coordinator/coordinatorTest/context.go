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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/caching/cacheContext"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/logging/memlogger"
	configPB "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/impl/resolving"
	"go.chromium.org/luci/config/vars"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/appengine/coordinator/flex"
	"go.chromium.org/luci/logdog/common/storage/archive"
	"go.chromium.org/luci/logdog/common/storage/bigtable"
	logdogcfg "go.chromium.org/luci/logdog/server/config"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
	"go.chromium.org/luci/server/caching"

	gaeMemory "go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"

	"github.com/golang/protobuf/proto"
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
	// ServiceID is LogDog's service ID for tests.
	ServiceID string

	// Clock is the installed test clock instance.
	Clock testclock.TestClock

	// AuthState is the fake authentication state.
	AuthState authtest.FakeState

	// Services is the set of installed Coordinator services.
	Services Services

	// BigTable in-memory testing instance.
	BigTable bigtable.Testing
	// GSClient is the test GSClient instance installed (by default) into
	// Services.
	GSClient GSClient

	// StorageCache is the default storage cache instance.
	StorageCache StorageCache

	// config is the luci-config configuration map that is installed.
	config map[config.Set]memory.Files
	// syncConfig moves configs in `config` into the datastore.
	syncConfig func()
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

// ModServiceConfig loads the current service configuration, invokes the
// callback with its contents, and writes the result back to config.
func (e *Environment) ModServiceConfig(c context.Context, fn func(*svcconfig.Config)) {
	var cfg svcconfig.Config
	e.modTextProtobuf(c, config.ServiceSet(e.ServiceID), "services.cfg", &cfg, func() {
		fn(&cfg)
	})
}

// ModProjectConfig loads the current configuration for the named project,
// invokes the callback with its contents, and writes the result back to config.
func (e *Environment) ModProjectConfig(c context.Context, project string, fn func(*svcconfig.ProjectConfig)) {
	var pcfg svcconfig.ProjectConfig
	e.modTextProtobuf(c, config.ProjectSet(project), e.ServiceID+".cfg", &pcfg, func() {
		fn(&pcfg)
	})
}

func (e *Environment) modTextProtobuf(c context.Context, configSet config.Set, path string,
	msg proto.Message, fn func()) {
	existing := e.config[configSet][path]
	if existing != "" {
		if err := proto.UnmarshalText(existing, msg); err != nil {
			panic(err)
		}
	}
	fn()
	e.addConfigEntry(configSet, path, proto.MarshalTextString(msg))
}

func (e *Environment) addConfigEntry(configSet config.Set, path, content string) {
	cset := e.config[configSet]
	if cset == nil {
		cset = make(memory.Files)
		e.config[configSet] = cset
	}
	cset[path] = content
	e.syncConfig()
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
		ServiceID: "logdog-app-id",
		GSClient:  GSClient{},
		StorageCache: StorageCache{
			Base: &flex.StorageCache{},
		},
		config: make(map[config.Set]memory.Files),
	}

	// Get our starting context.
	c := gaeMemory.UseWithAppID(memlogger.Use(context.Background()), e.ServiceID)
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC.Round(time.Millisecond))
	c = cryptorand.MockForTest(c, 765589025) // as chosen by fair dice roll
	ds.GetTestable(c).Consistent(true)

	// Signer is used by ShouldEnforceRealmACL to discover service ID.
	c = auth.ModifyConfig(c, func(cfg auth.Config) auth.Config {
		cfg.Signer = signingtest.NewSigner(&signing.ServiceInfo{AppID: e.ServiceID})
		return cfg
	})

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

	// Setup luci-config configuration.
	varz := vars.VarSet{}
	varz.Register("appid", func(context.Context) (string, error) {
		return e.ServiceID, nil
	})
	c = cfgclient.Use(c, resolving.New(&varz, memory.New(e.config)))

	// Capture the context while it doesn't have a lot of other stuff to use it
	// for Sync. We do it to simulate a sync done from the cron. The context
	// doesn't have a lot of stuff there.
	syncCtx := c
	e.syncConfig = func() { logdogcfg.Sync(syncCtx) }

	c = logdogcfg.WithStore(c, &logdogcfg.Store{NoCache: true})

	// luci-config: Projects.
	addProjectConfig := func(project string, access ...string) {
		projectAccesses := make([]string, len(access))

		// Build our service config. Also builds "projectAccesses".
		e.ModProjectConfig(c, project, func(pcfg *svcconfig.ProjectConfig) {
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
		e.modTextProtobuf(c, config.ProjectSet(project), "project.cfg", &pcfg, func() {
			pcfg = configPB.ProjectCfg{
				Name:   project,
				Access: projectAccesses,
			}
		})
	}
	addProjectConfig(AllAccessProject, "all:R", "all:W")
	addProjectConfig("proj-bar", "all:R", "auth:W")
	addProjectConfig("proj-exclusive", "auth:R", "auth:W")

	// Add a project without a LogDog project config.
	e.addConfigEntry("projects/proj-unconfigured", "not-logdog.cfg", "junk")

	// Add a project with malformed configs.
	e.addConfigEntry(config.ProjectSet("proj-malformed"), e.ServiceID+".cfg", "!!! not a text protobuf !!!")

	// luci-config: Coordinator Defaults
	e.ModServiceConfig(c, func(cfg *svcconfig.Config) {
		cfg.Transport = &svcconfig.Transport{
			Type: &svcconfig.Transport_Pubsub{
				Pubsub: &svcconfig.Transport_PubSub{
					Project: e.ServiceID,
					Topic:   "test-topic",
				},
			},
		}
		cfg.Coordinator = &svcconfig.Coordinator{
			AdminAuthGroup:   "admin",
			ServiceAuthGroup: "services",
			PrefixExpiration: durationpb.New(24 * time.Hour),
		}
	})

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
	}
	c = flex.WithServices(c, &e.Services)

	return cacheContext.Wrap(c), &e
}

// WithProjectNamespace runs f in project's namespace.
func WithProjectNamespace(c context.Context, project string, f func(context.Context)) {
	if err := coordinator.WithProjectNamespace(&c, project); err != nil {
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
