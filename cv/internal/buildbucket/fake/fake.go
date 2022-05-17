// Copyright 2022 The LUCI Authors.
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

package bbfake

import (
	"fmt"
	"sync"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/cv/internal/buildbucket"
)

type fakeApp struct {
	buildStoreMu sync.RWMutex
	buildStore   map[int64]*bbpb.Build // build ID -> build
}

type Fake struct {
	hostsMu sync.Mutex
	hosts   map[string]*fakeApp // hostname -> fakeApp
}

func (f *Fake) NewClientFactory() buildbucket.ClientFactory {
	return clientFactory{
		fake: f,
	}
}

// AddBuild adds a build to fake Buildbucket host.
//
// Reads Buildbucket hostname from `infra.buildbucket.hostname`.
// Overwrites the existing build if the build with same ID already exists.
func (f *Fake) AddBuild(build *bbpb.Build) {
	host := build.GetInfra().GetBuildbucket().GetHostname()
	if host == "" {
		panic(fmt.Errorf("missing host for build %d", build.Id))
	}
	fa := f.ensureApp(host)
	fa.buildStoreMu.Lock()
	fa.buildStore[build.GetId()] = build
	fa.buildStoreMu.Unlock()
}

func (f *Fake) ensureApp(host string) *fakeApp {
	f.hostsMu.Lock()
	defer f.hostsMu.Unlock()
	if _, ok := f.hosts[host]; !ok {
		if f.hosts == nil {
			f.hosts = make(map[string]*fakeApp)
		}
		f.hosts[host] = &fakeApp{
			buildStore: make(map[int64]*bbpb.Build),
		}
	}
	return f.hosts[host]
}

func (fa *fakeApp) getBuild(id int64) *bbpb.Build {
	fa.buildStoreMu.RLock()
	defer fa.buildStoreMu.RUnlock()
	if build, ok := fa.buildStore[id]; ok {
		return proto.Clone(build).(*bbpb.Build)
	}
	return nil
}

func (fa *fakeApp) iterBuildStore(cb func(*bbpb.Build)) {
	fa.buildStoreMu.RLock()
	defer fa.buildStoreMu.RUnlock()
	for _, build := range fa.buildStore {
		cb(proto.Clone(build).(*bbpb.Build))
	}
}

func (fa *fakeApp) updateBuild(id int64, cb func(*bbpb.Build)) *bbpb.Build {
	fa.buildStoreMu.Lock()
	defer fa.buildStoreMu.Unlock()
	if build, ok := fa.buildStore[id]; ok {
		cb(build)
		// store a copy to avoid cb keeps the reference to the build and mutate it
		// later.
		fa.buildStore[id] = proto.Clone(build).(*bbpb.Build)
		return build
	}
	return nil
}
