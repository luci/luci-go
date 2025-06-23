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
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
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
		sw.Task = &swarmingpb.NewTaskRequest{}
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

func (swe *swarmingEditor) tweakSlices(fn func(*swarmingpb.TaskSlice) error) {
	swe.tweak(func() error {
		for _, slice := range swe.sw.GetTask().GetTaskSlices() {
			if slice.Properties == nil {
				slice.Properties = &swarmingpb.TaskProperties{}
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
	swe.tweakSlices(func(slc *swarmingpb.TaskSlice) error {
		slc.Properties.CasInputRoot = nil
		return nil
	})
}

func (swe *swarmingEditor) ClearDimensions() {
	swe.tweakSlices(func(slc *swarmingpb.TaskSlice) error {
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
			*swarmingpb.TaskSlice
		}, len(slices))

		for i, slc := range slices {
			sliceRelativeExpiration := time.Duration(float64(slc.GetExpirationSecs()) * float64(time.Second))
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

				return errors.Fmt("%s%s%s@%d has invalid expiration time: current slices expire at %v",
					key, op, value.Value, value.Expiration/time.Second, validExpirations)
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
				slc.Properties = &swarmingpb.TaskProperties{}
			}
			dimMap := logicalDimensions{}
			for _, dim := range slc.Properties.Dimensions {
				dimMap.updateDuration(dim.Key, dim.Value, slc.TotalExpiration)
			}
			dimEdits.apply(dimMap, slc.TotalExpiration)
			newDims := make([]*swarmingpb.StringPair, 0, len(dimMap))
			for _, key := range keysOf(dimMap) {
				for _, value := range keysOf(dimMap[key]) {
					newDims = append(newDims, &swarmingpb.StringPair{
						Key: key, Value: value,
					})
				}
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

	swe.tweakSlices(func(slc *swarmingpb.TaskSlice) error {
		updateStringPairList(&slc.Properties.Env, env)
		return nil
	})
}

func (swe *swarmingEditor) Priority(priority int32) {
	swe.tweak(func() error {
		if priority < 0 {
			return errors.Fmt("negative Priority argument: %d", priority)
		}
		if task := swe.sw.GetTask(); task == nil {
			swe.sw.Task = &swarmingpb.NewTaskRequest{}
		}
		swe.sw.Task.Priority = priority
		return nil
	})
}

func (swe *swarmingEditor) CIPDPkgs(cipdPkgs CIPDPkgs) {
	swe.tweakSlices(func(slc *swarmingpb.TaskSlice) error {
		if slc.Properties.CipdInput == nil {
			slc.Properties.CipdInput = &swarmingpb.CipdInput{}
		}
		cipdPkgs.updateCipdPkgs(&slc.Properties.CipdInput.Packages)
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

func updatePrefixPathEnv(values []string, prefixes *[]*swarmingpb.StringListPair) {
	var pair *swarmingpb.StringListPair
	for _, pair = range *prefixes {
		if pair.Key == "PATH" {
			newPath := make([]string, len(pair.Value))
			copy(newPath, pair.Value)
			pair.Value = newPath
			break
		}
	}
	if pair == nil {
		pair = &swarmingpb.StringListPair{Key: "PATH"}
		*prefixes = append(*prefixes, pair)
	}

	var newPath []string
	for _, pair := range *prefixes {
		if pair.Key == "PATH" {
			newPath = make([]string, len(pair.Value))
			copy(newPath, pair.Value)
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

	pair.Value = newPath
}

func (swe *swarmingEditor) PrefixPathEnv(values []string) {
	if len(values) == 0 {
		return
	}

	swe.tweakSlices(func(slc *swarmingpb.TaskSlice) error {
		updatePrefixPathEnv(values, &slc.Properties.EnvPrefixes)
		return nil
	})
}

func validateTags(tags []string) error {
	for _, tag := range tags {
		if !strings.Contains(tag, ":") {
			return errors.Fmt("bad tag %q: must be in the form 'key:value'", tag)
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
				swe.sw.Task = &swarmingpb.NewTaskRequest{}
			}
			swe.sw.Task.Tags = append(swe.sw.Task.Tags, values...)
			sort.Strings(swe.sw.Task.Tags)
		}
		return
	})
}
