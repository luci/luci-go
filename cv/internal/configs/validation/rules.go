// Copyright 2018 The LUCI Authors.
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

package validation

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/info"
)

// Config validation rules go here.

func init() {
	addRules(&validation.Rules)
}

// TODO(crbug.com/1252545): Use a dev-specific configuration for a dev instance
// of the service after CQD is deleted.
func addRules(r *validation.RuleSet) {
	r.Vars.Register("cqCfgName", func(ctx context.Context) (string, error) {
		if appID := info.AppID(ctx); strings.HasSuffix(appID, "dev") {
			return "commit-queue-dev.cfg", nil
		}
		return "commit-queue.cfg", nil
	})
	r.Add("regex:projects/[^/]+", "${cqCfgName}", validateProject)
	r.Add("regex:projects/[^/]+", "${appid}.cfg", validateProject)
	r.Add("services/${appid}", "listener-settings.cfg", validateListenerSettings)
}

// regexpCompileCached is the caching version of regexp.Compile.
//
// Most config files use the same regexp many times.
func regexpCompileCached(pattern string) (*regexp.Regexp, error) {
	cached, err := regexpCache.GetOrCreate(context.Background(), pattern, func() (regexpCacheValue, time.Duration, error) {
		r, err := regexp.Compile(pattern)
		return regexpCacheValue{r, err}, 0, nil
	})
	if err != nil {
		return nil, err
	}
	return cached.r, cached.err
}

var regexpCache = lru.New[string, regexpCacheValue](1024)

type regexpCacheValue struct {
	r   *regexp.Regexp
	err error
}

func enter(vctx *validation.Context, kind string, i int, name string) {
	if name == "" {
		vctx.Enter(kind+" #%d", i+1)
	} else {
		vctx.Enter(kind+" #%d %q", i+1, name)
	}
}
