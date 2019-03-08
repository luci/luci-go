// Copyright 2017 The LUCI Authors.
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

package policy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/textproto"
)

// LuciConfigFetcher implements ConfigFetcher interface via LUCI Config client.
//
// It fetches all config files at a single revision: the first config file
// is fetched at HEAD, and the rest of them are fetched at the same revision.
type LuciConfigFetcher struct {
	m    sync.Mutex
	revs map[config.Set]string
}

// NewLuciConfigFetcher creates a new LUCI config fetcher.
func NewLuciConfigFetcher() *LuciConfigFetcher {
	return &LuciConfigFetcher{revs: make(map[config.Set]string, 0)}
}

// Revision returns the revision associated with the current config set.
func (f *LuciConfigFetcher) Revision(c context.Context) string {
	configSet := cfgclient.CurrentServiceConfigSet(c)
	f.m.Lock()
	defer f.m.Unlock()
	return f.revs[configSet]
}

// FetchTextProtoFromService fetches a config via particular config set.
func (f *LuciConfigFetcher) FetchTextProtoFromService(c context.Context, configSet config.Set, path string, out proto.Message) error {
	logging.Infof(c, "Reading %q from config set %q", path, configSet)
	c, cancel := context.WithTimeout(c, 40*time.Second) // URL fetch deadline
	defer cancel()

	var meta config.Meta
	if err := cfgclient.Get(c, cfgclient.AsService, configSet, path, textproto.Message(out), &meta); err != nil {
		return err
	}

	f.m.Lock()
	defer f.m.Unlock()

	switch {
	case f.revs[configSet] == "":
		f.revs[configSet] = meta.Revision
	case f.revs[configSet] != meta.Revision:
		// TODO(vadimsh): Lock all subsequent 'FetchTextProto' calls to the revision
		// we fetched during the first call. Unfortunately cfgclient doesn't support
		// fetching a specific revision. So we just fail if a wrong revision is
		// fetched, assuming the state will "normalize" later.
		return fmt.Errorf(
			"expected config %q to be at rev %s, but got %s",
			path, f.revs[configSet], meta.Revision)
	}

	return nil
}

// FetchTextProto fetches a config from the current config set.
func (f *LuciConfigFetcher) FetchTextProto(c context.Context, path string, out proto.Message) error {
	return f.FetchTextProtoFromService(c, cfgclient.CurrentServiceConfigSet(c), path, out)
}
