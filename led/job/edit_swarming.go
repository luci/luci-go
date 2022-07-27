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

	"go.chromium.org/luci/common/errors"
	api "go.chromium.org/luci/swarming/proto/api"
)

type swarmingEditor struct {
	jd *Definition
	sw *Swarming

	err error
}

var _ Editor = (*swarmingEditor)(nil)

func newSwarmingEditor(jd *Definition) *swarmingEditor {
	sw := jd.GetSwarming()
	if sw == nil {
		panic(errors.New("impossible: only supported for Swarming builds"))
	}
	if sw.Task == nil {
		sw.Task = &api.TaskRequest{}
	}

	return &swarmingEditor{jd, sw, nil}
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
		swe.jd.GetSwarming().CasUserPayload = nil

		return nil
	})
	swe.tweakSlices(func(slc *api.TaskSlice) error {
		slc.Properties.CasInputs = nil
		slc.Properties.CasInputRoot = nil
		return nil
	})
}

func (swe *swarmingEditor) ClearDimensions() {
	swe.tweakSlices(func(slc *api.TaskSlice) error {
		slc.Properties.Dimensions = nil
		return nil
	})
}

func (swe *swarmingEditor) SetDimensions(dims ExpiringDimensions) {
	swe.ClearDimensions()

	dec := DimensionEditCommands{}
	for key, vals := range dims {
		dec[key] = &DimensionEditCommand{SetValues: vals}
	}
	swe.EditDimensions(dec)
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
func (swe *swarmingEditor) EditDimensions(dimEdits DimensionEditCommands) {
	if len(dimEdits) == 0 {
		return
	}

	swe.tweak(func() error {
		taskRelativeExpirationSet := map[time.Duration]struct{}{}
		slices := swe.sw.GetTask().GetTaskSlices()
		sliceByExp := make([]struct {
			// seconds from start-of-task to expiration of this slice.
			TotalExpiration time.Duration
			*api.TaskSlice
		}, len(slices))

		for i, slc := range slices {
			if err := slc.Expiration.CheckValid(); err != nil {
				return err
			}
			sliceRelativeExpiration := slc.Expiration.AsDuration()
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
	swe.tweak(func() error {
		if priority < 0 {
			return errors.Reason("negative Priority argument: %d", priority).Err()
		}
		if task := swe.sw.GetTask(); task == nil {
			swe.sw.Task = &api.TaskRequest{}
		}
		swe.sw.Task.Priority = priority
		return nil
	})
}

func (swe *swarmingEditor) CIPDPkgs(cipdPkgs CIPDPkgs) {
	swe.tweakSlices(func(slc *api.TaskSlice) error {
		cipdPkgs.updateCipdPkgs(&slc.Properties.CipdInputs)
		return nil
	})
}

func (swe *swarmingEditor) SwarmingHostname(host string) {
	swe.tweak(func() (err error) {
		if host == "" {
			return errors.New("empty SwarmingHostname")
		}
		swe.sw.Hostname = host
		return
	})
}

func (swe *swarmingEditor) TaskName(name string) {
	swe.tweak(func() (err error) {
		swe.sw.Task.Name = name
		return
	})
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
	for _, tag := range tags {
		if !strings.Contains(tag, ":") {
			return errors.Reason("bad tag %q: must be in the form 'key:value'", tag).Err()
		}
	}
	return nil
}

func (swe *swarmingEditor) Tags(values []string) {
	if len(values) == 0 {
		return
	}
	swe.tweak(func() (err error) {
		if err = validateTags(values); err == nil {
			if swe.sw.Task == nil {
				swe.sw.Task = &api.TaskRequest{}
			}
			swe.sw.Task.Tags = append(swe.sw.Task.Tags, values...)
			sort.Strings(swe.sw.Task.Tags)
		}
		return
	})
}
