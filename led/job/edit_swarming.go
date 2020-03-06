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
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
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
	swm.tweakSlices(func(slc *api.TaskSlice) error {
		slc.Properties.Dimensions = nil
		return nil
	})
}

func (swm *swarmingEditor) SetDimensions(dims ExpiringDimensions) {
	swm.ClearDimensions()

	dec := DimensionEditCommands{}
	for key, vals := range dims {
		dec[key] = &DimensionEditCommand{SetValues: vals}
	}
	swm.EditDimensions(dec)
}

// EditDimensions is a bit trickier for swarming than it is for buildbucket.
//
// We want to map the dimEdits onto existing slices; Slices in the swarming task
// are listed with their expiration times relative to the previous slice, which
// means we need to do a bit of precomputation to convert these to
// expiration-relative-to-task-start times.
//
// If dimEdits contains set/add values which don't align with any existing
// slices, this will set an error.
func (swm *swarmingEditor) EditDimensions(dimEdits DimensionEditCommands) {
	if len(dimEdits) == 0 {
		return
	}

	swm.tweak(func() error {
		taskRelativeExpirationSet := map[time.Duration]struct{}{}
		slices := swm.sw.GetTask().GetTaskSlices()
		sliceByExp := make([]struct {
			// seconds from start-of-task to expiration of this slice.
			TotalExpiration time.Duration
			*api.TaskSlice
		}, len(slices))

		for i, slc := range slices {
			sliceRelativeExpiration, err := ptypes.Duration(slc.Expiration)
			if err != nil {
				return err
			}
			taskRelativeExpiration := sliceRelativeExpiration
			if i > 0 {
				taskRelativeExpiration += sliceByExp[i-1].TotalExpiration
			}
			taskRelativeExpirationSet[taskRelativeExpiration] = struct{}{}

			sliceByExp[i].TotalExpiration = taskRelativeExpiration
			sliceByExp[i].TaskSlice = slc
		}

		checkValidExpiration := func(key string, value ExpiringValue, op string) error {
			if value.Expiration == 0 {
				return nil
			}

			if _, ok := taskRelativeExpirationSet[value.Expiration]; !ok {
				validExpirations := make([]int64, len(sliceByExp)+1)
				for i, slc := range sliceByExp {
					validExpirations[i+1] = int64(slc.TotalExpiration / time.Second)
				}

				return errors.Reason(
					"%s%s%s@%d has invalid expiration time: current slices expire at %v",
					key, op, value.Value, value.Expiration/time.Second, validExpirations).Err()
			}
			return nil
		}

		for key, edits := range dimEdits {
			for _, setval := range edits.SetValues {
				if err := checkValidExpiration(key, setval, "="); err != nil {
					return err
				}
			}
			for _, addval := range edits.AddValues {
				if err := checkValidExpiration(key, addval, "+="); err != nil {
					return err
				}
			}
		}

		// Now we know that all the edits slot into some slice, we can actually
		// apply them.
		for _, slc := range sliceByExp {
			if slc.Properties == nil {
				slc.Properties = &api.TaskProperties{}
			}
			dimMap := logicalDimensions{}
			for _, dim := range slc.Properties.Dimensions {
				for _, value := range dim.Values {
					dimMap.updateDuration(dim.Key, value, slc.TotalExpiration)
				}
			}
			dimEdits.apply(dimMap, slc.TotalExpiration)
			newDims := make([]*api.StringListPair, 0, len(dimMap))
			for _, key := range keysOf(dimMap) {
				values := dimMap[key]
				newDims = append(newDims, &api.StringListPair{
					Key: key, Values: values.toSlice(),
				})
			}
			slc.Properties.Dimensions = newDims
		}

		return nil
	})
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
	if priority < 0 {
		return
	}
	swm.tweak(func() error {
		if task := swm.sw.GetTask(); task == nil {
			swm.sw.Task = &api.TaskRequest{}
		}
		swm.sw.Task.Priority = priority
		return nil
	})
}

func (swm *swarmingEditor) CIPDPkgs(cipdPkgs CIPDPkgs) {
	swm.tweakSlices(func(slc *api.TaskSlice) error {
		cipdPkgs.updateCipdPkgs(&slc.Properties.CipdInputs)
		return nil
	})
}

func (swm *swarmingEditor) SwarmingHostname(host string) {
	if host == "" {
		return
	}

	swm.tweak(func() (err error) {
		swm.sw.Hostname = host
		return
	})
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
	for _, tag := range tags {
		if !strings.Contains(tag, ":") {
			return errors.Reason("bad tag %q: must be in the form 'key:value'", tag).Err()
		}
	}
	return nil
}

func (swm *swarmingEditor) Tags(values []string) {
	if len(values) == 0 {
		return
	}
	swm.tweak(func() (err error) {
		if err = validateTags(values); err == nil {
			if swm.sw.Task == nil {
				swm.sw.Task = &api.TaskRequest{}
			}
			swm.sw.Task.Tags = append(swm.sw.Task.Tags, values...)
			sort.Strings(swm.sw.Task.Tags)
		}
		return
	})
}
