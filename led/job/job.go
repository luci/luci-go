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
	"context"
	"encoding/hex"
	"path"
	"sort"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	durpb "github.com/golang/protobuf/ptypes/duration"

	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	logdog_types "go.chromium.org/luci/logdog/common/types"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

type isoInput struct {
	Server    string `json:"server"`
	Namespace string `json:"namespace"`
	Hash      string `json:"hash"`
}

type cipdInput struct {
	Package string `json:"package"`
	Version string `json:"version"`
}

type ledProperties struct {
	LedRunID string `json:"led_run_id"`

	IsolatedInput *isoInput `json:"isolated_input,omitempty"`

	CIPDInput *cipdInput `json:"cipd_input,omitempty"`
}

func (jd *Definition) addLedProperties(ctx context.Context, uid string) error {
	// Set the "$recipe_engine/led" recipe properties.
	buf := make([]byte, 32)
	if _, err := cryptorand.Read(ctx, buf); err != nil {
		return errors.Annotate(err, "generating random token").Err()
	}
	streamName, err := logdog_types.MakeStreamName("", "led", uid, hex.EncodeToString(buf))
	if err != nil {
		return errors.Annotate(err, "generating logdog token").Err()
	}
	logdogPrefix := string(streamName)

	bb := jd.GetBuildbucket()
	if bb == nil {
		panic("impossible: Buildbucket is nil while flattening to swarming")
	}
	bb.EnsureBasics()

	// TODO(iannucci): change logdog project to something reserved to 'led' tasks.
	// Though if we merge logdog into resultdb, this hopefully becomes moot.
	bb.BbagentArgs.Build.Infra.Logdog.Prefix = logdogPrefix

	// Pass the CIPD package or isolate containing the recipes code into
	// the led recipe module. This gives the build the information it needs
	// to launch child builds using the same version of the recipes code.
	props := ledProperties{LedRunID: logdogPrefix}

	// The logdog prefix is unique to each led job, so it can be used as an
	// ID for the job.
	if payload := jd.GetUserPayload(); payload.GetDigest() != "" {
		props.IsolatedInput = &isoInput{
			Server:    payload.GetServer(),
			Namespace: payload.GetNamespace(),
			Hash:      payload.GetDigest(),
		}
	} else if pkg := bb.GetBbagentArgs().GetBuild().GetExe(); pkg != nil {
		props.CIPDInput = &cipdInput{
			Package: pkg.GetCipdPackage(),
			Version: pkg.GetCipdVersion(),
		}
	}

	bb.WriteProperties(map[string]interface{}{
		"$recipe_engine/led": props,
	})

	logdogTag := "log_location:logdog://logs.chromium.org/" + logdogPrefix
	if bb.LegacyKitchen {
		logdogTag += "/+/annotations"
	} else {
		logdogTag += "/+/build.proto"
	}

	return jd.Edit(func(je Editor) {
		je.Tags([]string{logdogTag, "allow_milo:1"})
	})
}

type expiringData struct {
	absolute time.Duration // from scheduling task
	relative time.Duration // from previous slice

	dimesions []*swarmingpb.StringPair
	caches    []*bbpb.BuildInfra_Swarming_CacheEntry
}

func (ed *expiringData) createWith(cacheDir string, template *swarmingpb.TaskProperties) *swarmingpb.TaskProperties {
	if len(template.Dimensions) != 0 {
		panic("impossible; createWith called with dimensions already set")
	}

	ret := proto.Clone(template).(*swarmingpb.TaskProperties)

	dimMap := map[string]stringset.Set{}
	addDims := func(key string, values ...string) {
		if set, ok := dimMap[key]; !ok {
			dimMap[key] = stringset.NewFromSlice(values...)
		} else {
			set.AddAll(values)
		}
	}

	for _, dim := range ed.dimesions {
		addDims(dim.Key, dim.Value)
	}
	for _, nc := range ed.caches {
		ret.NamedCaches = append(ret.NamedCaches, &swarmingpb.NamedCacheEntry{
			Name:     nc.Name,
			DestPath: path.Join(cacheDir, nc.Path),
		})
		addDims("caches", nc.Name)
	}

	newDims := make([]*swarmingpb.StringListPair, 0, len(dimMap))
	for _, key := range keysOf(dimMap) {
		newDims = append(newDims, &swarmingpb.StringListPair{
			Key: key, Values: dimMap[key].ToSortedSlice()})
	}
	ret.Dimensions = newDims

	return ret
}

func (jd *Definition) makeExpiringSliceData() (ret []*expiringData, err error) {
	bb := jd.GetBuildbucket()
	expirationSet := map[time.Duration]*expiringData{}
	nonExpiring := expiringData{}
	addExpiration := func(name string, protoDuration *durpb.Duration, add func(d *expiringData)) error {
		if protoDuration == nil {
			protoDuration = &durpb.Duration{}
		}
		dur, err := ptypes.Duration(protoDuration)
		if err != nil {
			return errors.Annotate(err, "parsing %s expiration", name).Err()
		}

		if dur > 0 {
			data, ok := expirationSet[dur]
			if !ok {
				data = &expiringData{absolute: dur}
				expirationSet[dur] = data
			}
			add(data)
		} else {
			add(&nonExpiring)
		}
		return nil
	}
	for _, cache := range bb.BbagentArgs.GetBuild().GetInfra().GetSwarming().GetCaches() {
		err := addExpiration("cache", cache.WaitForWarmCache, func(data *expiringData) {
			data.caches = append(data.caches, cache)
		})
		if err != nil {
			return nil, err
		}
	}
	for _, dim := range bb.BbagentArgs.GetBuild().GetInfra().GetSwarming().GetTaskDimensions() {
		err := addExpiration("dimension", dim.Expiration, func(data *expiringData) {
			data.dimesions = append(data.dimesions, &swarmingpb.StringPair{Key: dim.Key, Value: dim.Value})
		})
		if err != nil {
			return nil, err
		}
	}

	ret = make([]*expiringData, 0, len(expirationSet))
	for _, data := range expirationSet {
		ret = append(ret, data)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].absolute < ret[j].absolute
	})
	ret[0].relative = ret[0].absolute
	for i := range ret[1:] {
		ret[i+1].relative = ret[i+1].absolute - ret[i].absolute
	}
	if total, err := ptypes.Duration(bb.BbagentArgs.Build.SchedulingTimeout); err == nil {
		if ret[len(ret)-1].absolute < total {
			// if the task's total expiration time is greater than the last slice's
			// expiration, then use nonExpiring as the last slice.
			nonExpiring.absolute = total
			nonExpiring.relative = total - ret[len(ret)-1].absolute
			ret = append(ret, &nonExpiring)
		} else {
			// otherwise, add all of nonExpiring's guts to the last slice.
			last := ret[len(ret)-1]
			last.caches = append(last.caches, nonExpiring.caches...)
			last.dimesions = append(last.dimesions, nonExpiring.dimesions...)
		}
	}

	// Ret now looks like:
	//   rel @ 20s - caches:[a b c]
	//   rel @ 40s - caches:[d e]
	//   rel @ inf - caches:[f]
	//
	// We need to transform this into:
	//   rel @ 20s - caches:[a b c d e f]
	//   rel @ 40s - caches:[d e f]
	//   rel @ inf - caches:[f]
	//
	// Since a slice expiring at 20s includes all the caches (and dimensions) of
	// all slices expiring after it.

	for i := len(ret) - 2; i >= 0; i-- {
		ret[i].dimesions = append(ret[i].dimesions, ret[i+1].dimesions...)
		ret[i].caches = append(ret[i].caches, ret[i+1].caches...)
	}

	return
}

func (jd *Definition) generateCommand(ctx context.Context, ks KitchenSupport) ([]string, error) {
	bb := jd.GetBuildbucket()

	if bb.LegacyKitchen {
		return ks.GenerateCommand(ctx, bb)
	}

	// TODO(iannucci): have bbagent set 'logdog.viewer_url' to the milo build
	// view URL if there's no buildbucket build associated with it.

	return []string{
		"bbagent${EXECUTABLE_SUFFIX}", bbinput.Encode(bb.BbagentArgs),
	}, nil
}

// FlattenToSwarming modifies this Definition to populate the Swarming field
// from the Buildbucket field.
//
// After flattening, HighLevelEdit functionality will no longer work on this
// Definition.
func (jd *Definition) FlattenToSwarming(ctx context.Context, uid string, ks KitchenSupport) error {
	if jd.GetSwarming() != nil {
		return nil
	}

	err := jd.addLedProperties(ctx, uid)
	if err != nil {
		return errors.Annotate(err, "adding led properties").Err()
	}

	expiringData, err := jd.makeExpiringSliceData()
	if err != nil {
		return errors.Annotate(err, "calculating expirations").Err()
	}

	bb := jd.GetBuildbucket()
	bbi := bb.GetBbagentArgs().GetBuild().GetInfra()
	sw := &Swarming{
		Hostname: jd.Info().SwarmingHostname(),
		Task: &swarmingpb.TaskRequest{
			Name:           jd.Info().TaskName(),
			Priority:       jd.Info().Priority(),
			ServiceAccount: bbi.GetSwarming().GetTaskServiceAccount(),
			Tags:           jd.Info().Tags(),
			User:           uid,
			TaskSlices:     make([]*swarmingpb.TaskSlice, len(expiringData)),
		},
	}

	baseProperties := &swarmingpb.TaskProperties{
		CipdInputs: append(([]*swarmingpb.CIPDPackage)(nil), bb.CipdPackages...),
		CasInputs:  jd.UserPayload,

		EnvPaths:         bb.EnvPrefixes,
		ExecutionTimeout: bb.BbagentArgs.Build.ExecutionTimeout,
		GracePeriod:      bb.GracePeriod,

		Containment: bb.Containment,
	}

	baseProperties.Command, err = jd.generateCommand(ctx, ks)
	if err != nil {
		return errors.Annotate(err, "generating Command").Err()
	}

	if exe := bb.BbagentArgs.Build.Exe; exe != nil {
		baseProperties.CipdInputs = append(baseProperties.CipdInputs, &swarmingpb.CIPDPackage{
			PackageName: exe.CipdPackage,
			Version:     exe.CipdVersion,
			DestPath:    path.Dir(bb.BbagentArgs.ExecutablePath),
		})
	}

	for i, dat := range expiringData {
		sw.Task.TaskSlices[i] = &swarmingpb.TaskSlice{
			Expiration: ptypes.DurationProto(dat.relative),
			Properties: dat.createWith(bb.BbagentArgs.CacheDir, baseProperties),
		}
	}

	jd.JobType = &Definition_Swarming{Swarming: sw}
	return nil
}
