// Copyright 2020 The LUCI Authors.
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

package prjcfg

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"go.chromium.org/luci/gae/service/info"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
)

// GerritHost returns Gerrit host.
//
// Panics if config is not valid.
func GerritHost(cfg *cfgpb.ConfigGroup_Gerrit) string {
	u := cfg.GetUrl()
	if u == "" {
		panic("invalid config: missing Gerrit URL")
	}
	switch p, err := url.Parse(u); {
	case err != nil:
		panic(fmt.Errorf("invalid config: bad Gerrit URL %q: %s", u, err))
	case p.Scheme == "":
		// Catch "x.y.z", which url.Parse rightly interprets as Path, not Host.
		panic(fmt.Errorf("invalid config: bad Gerrit URL %q: missing scheme", u))
	default:
		return p.Hostname()
	}
}

// ConfigFileName returns the project config file name used by LUCI CV.
func ConfigFileName(ctx context.Context) string {
	if appid := info.AppID(ctx); strings.HasSuffix(appid, "dev") {
		return fmt.Sprintf("%s.cfg", appid)
	}
	return "commit-queue.cfg"
}
