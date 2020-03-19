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

package job

import (
	"strings"

	"go.chromium.org/luci/common/errors"
	api "go.chromium.org/luci/swarming/proto/api"
)

type swarmingEditor struct {
	jd          *Definition
	sw          *Swarming
	userPayload *api.CASTree

	err error
}

var _ Editor = (*swarmingEditor)(nil)

func newSwarmingEditor(jd *Definition) *swarmingEditor {
	sw := jd.GetSwarming()
	if sw == nil {
		panic(errors.New("impossible: only supported for Swarming builds"))
	}
	if jd.UserPayload == nil {
		jd.UserPayload = &api.CASTree{}
	}
	return &swarmingEditor{jd, sw, jd.UserPayload, nil}
}

func (swe *swarmingEditor) Close() error {
	return swe.err
}

func (swe *swarmingEditor) tweak(fn func() error) {
	if swe.err == nil {
		swe.err = fn()
	}
}

func (swe *swarmingEditor) tweakSlices(fn func(*api.TaskSlice) error) {
	swe.tweak(func() error {
		for _, slice := range swe.sw.GetTask().GetTaskSlices() {
			if slice.Properties == nil {
				slice.Properties = &api.TaskProperties{}
			}

			if err := fn(slice); err != nil {
				return err
			}
		}
		return nil
	})
}

func (swe *swarmingEditor) ClearCurrentIsolated() {
	swe.tweak(func() error {
		swe.userPayload.Digest = ""
		return nil
	})
	swe.tweakSlices(func(slc *api.TaskSlice) error {
		slc.Properties.CasInputs = nil
		return nil
	})
}

func (swe *swarmingEditor) ClearDimensions() {
	panic("implement me")
}

func (swe *swarmingEditor) Env(env map[string]string) {
	if len(env) == 0 {
		return
	}

	swe.tweakSlices(func(slc *api.TaskSlice) error {
		updateStringPairList(&slc.Properties.Env, env)
		return nil
	})
}

func (swe *swarmingEditor) Priority(priority int32) {
	panic("implement me")
}

func (swe *swarmingEditor) SwarmingHostname(host string) {
	panic("implement me")
}

func updatePrefixPathEnv(values []string, prefixes *[]*api.StringListPair) {
	var pair *api.StringListPair
	for _, pair = range *prefixes {
		if pair.Key == "PATH" {
			newPath := make([]string, len(pair.Values))
			copy(newPath, pair.Values)
			pair.Values = newPath
			break
		}
	}
	if pair == nil {
		pair = &api.StringListPair{Key: "PATH"}
		*prefixes = append(*prefixes, pair)
	}

	var newPath []string
	for _, pair := range *prefixes {
		if pair.Key == "PATH" {
			newPath = make([]string, len(pair.Values))
			copy(newPath, pair.Values)
			break
		}
	}

	for _, v := range values {
		if strings.HasPrefix(v, "!") {
			idx := 0
			for _, cur := range newPath {
				if cur != v[1:] {
					newPath[idx] = cur
					idx++
				}
			}
			newPath = newPath[:idx]
		} else {
			newPath = append(newPath, v)
		}
	}

	pair.Values = newPath
}

func (swe *swarmingEditor) PrefixPathEnv(values []string) {
	if len(values) == 0 {
		return
	}

	swe.tweakSlices(func(slc *api.TaskSlice) error {
		updatePrefixPathEnv(values, &slc.Properties.EnvPaths)
		return nil
	})
}

func validateTags(tags []string) error {
	panic("implement me")
}

func (swe *swarmingEditor) Tags(values []string) {
	panic("implement me")
}
