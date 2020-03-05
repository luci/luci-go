// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
	"strings"

	"go.chromium.org/luci/common/errors"
	api "go.chromium.org/luci/swarming/proto/api"
)

// swarmingEditor is a temporary type returned by Definition.Edit. It holds
// a mutable swarming-based Definition and an error, allowing a series of Edit
// commands to be called while buffering the error (if any).  Obtain the
// modified Definition (or error) by calling Finalize.
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

func (swm *swarmingEditor) Close() error {
	return swm.err
}

func (swm *swarmingEditor) tweak(fn func() error) {
	if swm.err == nil {
		swm.err = fn()
	}
}

func (swm *swarmingEditor) tweakSlices(fn func(*api.TaskSlice) error) {
	swm.tweak(func() error {
		for _, slice := range swm.sw.GetTask().GetTaskSlices() {
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

func (swm *swarmingEditor) ClearCurrentIsolated() {
	swm.tweak(func() error {
		swm.userPayload.Digest = ""
		return nil
	})
	swm.tweakSlices(func(slc *api.TaskSlice) error {
		slc.Properties.CasInputs = nil
		return nil
	})
}

func (swm *swarmingEditor) ClearDimensions() {
	panic("implement me")
}

func (swm *swarmingEditor) Env(env map[string]string) {
	if len(env) == 0 {
		return
	}

	swm.tweakSlices(func(slc *api.TaskSlice) error {
		updateStringPairList(&slc.Properties.Env, env)
		return nil
	})
}

func (swm *swarmingEditor) Priority(priority int32) {
	panic("implement me")
}

func (swm *swarmingEditor) SwarmingHostname(host string) {
	panic("implement me")
}

func updatePrefixPathEnv(values []string, prefixes *[]*api.StringListPair) {
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
			var toCut []int
			for i, cur := range newPath {
				if cur == v[1:] {
					toCut = append(toCut, i)
				}
			}
			for _, i := range toCut {
				newPath = append(newPath[:i], newPath[i+1:]...)
			}
		} else {
			newPath = append(newPath, v)
		}
	}

	for _, pair := range *prefixes {
		if pair.Key == "PATH" {
			pair.Values = newPath
			return
		}
	}

	*prefixes = append(
		*prefixes, &api.StringListPair{Key: "PATH", Values: newPath})
}

func (swm *swarmingEditor) PrefixPathEnv(values []string) {
	if len(values) == 0 {
		return
	}

	swm.tweakSlices(func(slc *api.TaskSlice) error {
		updatePrefixPathEnv(values, &slc.Properties.EnvPaths)
		return nil
	})
}

func validateTags(tags []string) error {
	panic("implement me")
}

func (swm *swarmingEditor) Tags(values []string) {
	panic("implement me")
}
