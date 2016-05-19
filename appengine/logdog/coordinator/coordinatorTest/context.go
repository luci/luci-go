// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinatorTest

import (
	"fmt"

	"github.com/golang/protobuf/proto"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	luciConfig "github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	configProto "github.com/luci/luci-go/common/proto/config"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"github.com/luci/luci-go/server/logdog/storage"
	memoryStorage "github.com/luci/luci-go/server/logdog/storage/memory"
	"github.com/luci/luci-go/server/settings"
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
	// ConfigIface is a reference to the memory config.Interface that Config is
	// installed into.
	ConfigIface luciConfig.Interface

	// Services is the set of installed Coordinator services.
	Services Services

	// IntermediateStorage is the memory Intermediate Storage instance
	// installed (by default) into Services.
	IntermediateStorage memoryStorage.Storage
	// GSClient is the test GSClient instance installed (by default) into
	// Services.
	GSClient GSClient
	// ArchivalPublisher is the test ArchivalPublisher instance installed (by
	// default) into Services.
	ArchivalPublisher ArchivalPublisher
}

// JoinGroup adds the named group the to the list of groups for the current
// identity.
func (e *Environment) JoinGroup(g string) {
	e.AuthState.IdentityGroups = append(e.AuthState.IdentityGroups, g)
}

// ClearCoordinatorConfig removes the Coordinator configuration entry,
// simulating a missing config.
func (e *Environment) ClearCoordinatorConfig(c context.Context) {
	configSet, _ := config.ServiceConfigPath(c)
	delete(e.Config, configSet)
}

// ModServiceConfig loads the current service configuration, invokes the
// callback with its contents, and writes the result back to config.
func (e *Environment) ModServiceConfig(c context.Context, fn func(*svcconfig.Coordinator)) {
	configSet, configPath := config.ServiceConfigPath(c)

	var cfg svcconfig.Config
	e.modTextProtobuf(configSet, configPath, &cfg, func() {
		if cfg.Coordinator == nil {
			cfg.Coordinator = &svcconfig.Coordinator{}
		}
		fn(cfg.Coordinator)
	})
}

// DrainTumbleAll drains all Tumble instances across all namespaces.
func (e *Environment) DrainTumbleAll(c context.Context) {
	projects, err := luciConfig.Get(c).GetProjects()
	if err != nil {
		panic(err)
	}

	for _, proj := range projects {
		WithProjectNamespace(c, luciConfig.ProjectName(proj.ID), func(c context.Context) {
			e.Tumble.Drain(c)
		})
	}
}

func (e *Environment) modTextProtobuf(configSet, path string, msg proto.Message, fn func()) {
	cfg, err := e.ConfigIface.GetConfig(configSet, path, false)

	switch err {
	case nil:
		if err := proto.UnmarshalText(cfg.Content, msg); err != nil {
			panic(err)
		}

	case luciConfig.ErrNoConfig:
		break

	default:
		panic(err)
	}

	fn()

	cset := e.Config[configSet]
	if cset == nil {
		cset = make(map[string]string)
		e.Config[configSet] = cset
	}
	cset[path] = proto.MarshalTextString(msg)
}

// Install creates a testing Context and installs common test facilities into
// it, returning the Environment to which they're bound.
func Install() (context.Context, *Environment) {
	e := Environment{
		Config: make(map[string]memory.ConfigSet),
	}

	// Get our starting context. This installs, among other things, in-memory
	// gae, settings, and logger.
	c := e.Tumble.Context()
	if *testGoLogger {
		c = logging.SetLevel(gologger.StdConfig.Use(c), logging.Debug)
	}

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
	ds.Get(c).Testable().AddIndexes(indexes...)

	// Setup clock.
	e.Clock = clock.Get(c).(testclock.TestClock)

	// Install GAE config service settings.
	c = settings.Use(c, settings.New(&settings.MemoryStorage{}))

	// Setup luci-config configuration.
	c = memory.Use(c, e.Config)
	e.ConfigIface = luciConfig.Get(c)

	// luci-config: Projects.
	addProjectConfig := func(proj luciConfig.ProjectName, localName string, access ...string) {
		configSet, configPath := config.ProjectConfigPath(c, proj)

		var cfg configProto.ProjectCfg
		e.modTextProtobuf(configSet, configPath, &cfg, func() {
			cfg.Name = &localName
			cfg.Access = access
		})
	}
	addProjectConfig("proj-foo", "Foo Project", "group:all")
	addProjectConfig("proj-bar", "Bar Project", "group:all")
	addProjectConfig("proj-baz", "Baz Project", "group:all")
	addProjectConfig("proj-qux", "Qux Project", "group:all")
	addProjectConfig("proj-exclusive", "Exclusive Project", "group:auth")

	// luci-config: Coordinator Defaults
	e.ModServiceConfig(c, func(cfg *svcconfig.Coordinator) {
		*cfg = svcconfig.Coordinator{
			AdminAuthGroup:   "admin",
			ServiceAuthGroup: "services",
		}
	})

	// Setup Tumble. This also adds the two Tumble indexes to datastore.
	e.Tumble.EnableDelayedMutations(c)

	tcfg := e.Tumble.GetConfig(c)
	tcfg.Namespaced = true
	e.Tumble.UpdateSettings(c, tcfg)

	// Install authentication state.
	c = auth.WithState(c, &e.AuthState)

	// Setup authentication state.
	e.JoinGroup("all")

	// Setup our default Coordinator services.
	e.Services = Services{
		IS: func() (storage.Storage, error) {
			return &e.IntermediateStorage, nil
		},
		GS: func() (gs.Client, error) {
			return &e.GSClient, nil
		},
		AP: func() (coordinator.ArchivalPublisher, error) {
			return &e.ArchivalPublisher, nil
		},
	}
	c = coordinator.WithServices(c, &e.Services)

	return c, &e
}

// WithProjectNamespace runs f in proj's namespace, bypassing authentication
// checks.
func WithProjectNamespace(c context.Context, proj luciConfig.ProjectName, f func(context.Context)) {
	if err := coordinator.WithProjectNamespaceNoAuth(&c, proj); err != nil {
		panic(err)
	}
	f(c)
}
