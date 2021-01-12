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

	protov1 "github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
)

// luciConfigFetcher implements ConfigFetcher interface via LUCI Config client.
//
// It fetches all config files at a single revision: the first config file
// is fetched at HEAD, and the rest of them are fetched at the same revision.
type luciConfigFetcher struct {
	m   sync.Mutex
	rev string
}

func (f *luciConfigFetcher) Revision() string {
	f.m.Lock()
	defer f.m.Unlock()
	return f.rev
}

func (f *luciConfigFetcher) FetchTextProto(c context.Context, path string, out proto.Message) error {
	logging.Infof(c, "Reading %q", path)
	c, cancel := context.WithTimeout(c, 40*time.Second) // URL fetch deadline
	defer cancel()

	var meta config.Meta
	err := cfgclient.Get(c,
		"services/${appid}",
		path,
		cfgclient.ProtoText(protov1.MessageV1(out)),
		&meta,
	)
	if err != nil {
		return err
	}

	f.m.Lock()
	defer f.m.Unlock()

	switch {
	case f.rev == "":
		f.rev = meta.Revision
	case f.rev != meta.Revision:
		// TODO(vadimsh): Lock all subsequent 'FetchTextProto' calls to the revision
		// we fetched during the first call. Unfortunately cfgclient doesn't support
		// fetching a specific revision. So we just fail if a wrong revision is
		// fetched, assuming the state will "normalize" later.
		return fmt.Errorf(
			"expected config %q to be at rev %s, but got %s",
			path, f.rev, meta.Revision)
	}

	return nil
}
