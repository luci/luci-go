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

package listener

import (
	"context"
	"regexp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/cv/internal/configs/srvcfg"
)

// IsPubsubEnabled returns whether listener is configured to process pubsub
// messages for a given project.
//
// The returned value only tells the intention, based on the config.
// It's possible that listener
// - is still processing pubsub messages even if this function returns false
// - stopped processing pubsub messages even if this function returns true
func IsPubsubEnabled(ctx context.Context, prj string) (bool, error) {
	cfg, err := srvcfg.GetListenerConfig(ctx, nil)
	if err != nil {
		return false, transient.Tag.Apply(err)
	}

	res, err := anchorRegexps(cfg.GetEnabledProjectRegexps())
	if err != nil {
		// The config validation shouldn't allow this to happen.
		return false, err
	}
	for _, re := range res {
		if re.Match([]byte(prj)) {
			return true, nil
		}
	}
	return false, nil
}

// anchorRegexps converts partial match regexs to full matches.
//
// Returns compiled regexps of them.
func anchorRegexps(partials []string) ([]*regexp.Regexp, error) {
	var me errors.MultiError
	var res []*regexp.Regexp
	for _, p := range partials {
		re, err := regexp.Compile("^" + p + "$")
		me.MaybeAdd(err)
		res = append(res, re)
	}
	if me.AsError() == nil {
		return res, nil
	}
	return nil, me
}
