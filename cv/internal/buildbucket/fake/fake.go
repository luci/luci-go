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

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"

	"go.chromium.org/luci/cv/internal/buildbucket"
)

type buildID struct {
	host string
	id   int64
}
type Fake struct {
	storeMu sync.RWMutex
	store   map[buildID]*buildbucketpb.Build
}

func (f *Fake) NewClientFactory() buildbucket.ClientFactory {
	return clientFactory{
		fake: f,
	}
}

func (f *Fake) AddBuild(build *buildbucketpb.Build) {
	host := build.GetInfra().GetBuildbucket().GetHostname()
	if host == "" {
		panic(fmt.Errorf("missing host for build %d", build.Id))
	}
	f.storeMu.Lock()
	defer f.storeMu.Unlock()
	if f.store == nil {
		f.store = make(map[buildID]*buildbucketpb.Build)
	}
	f.store[buildID{host, build.GetId()}] = build
}
