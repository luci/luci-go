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
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/proto"
	durpb "google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/cryptorand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/flagenum"
	logdog_types "go.chromium.org/luci/logdog/common/types"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/led/job/experiments"
)

type cipdInput struct {
	Package string `json:"package"`
	Version string `json:"version"`
}

type ledProperties struct {
	LedRunID string `json:"led_run_id"`

	RbeCasInput *swarmingpb.CASReference `json:"rbe_cas_input,omitempty"`

	CIPDInput *cipdInput `json:"cipd_input,omitempty"`

	ShadowedBucket string `json:"shadowed_bucket"`
}

// For accepting the "-resultdb" flag of "led launch".
type RDBEnablement string

func (r *RDBEnablement) String() string {
	return string(*r)
}

func (r *RDBEnablement) Set(v string) error {
	return RdbChoices.FlagSet(r, v)
}

const (
	// Swarming/ResultDB integration will be forcefully enabled.
	RDBOn RDBEnablement = "on"
	// Swarming/ResultDB integration will be forcefully disabled.
	RDBOff RDBEnablement = "off"
)

var RdbChoices = flagenum.Enum{
	"on":  RDBOn,
	"off": RDBOff,
}

func (jd *Definition) addLedProperties(ctx context.Context, uid string) (err error) {
	// Set the "$recipe_engine/led" recipe properties.
	bb := jd.GetBuildbucket()
	if bb == nil {
		panic("impossible: Buildbucket is nil while flattening to swarming")
	}
	bb.EnsureBasics()

	bb.BbagentArgs.Build.CreateTime, err = ptypes.TimestampProto(clock.Now(ctx))
	if err != nil {
		return errors.Fmt("populating creation time: %w", err)
	}

	buf := make([]byte, 32)
	if _, err := cryptorand.Read(ctx, buf); err != nil {
		return errors.Fmt("generating random token: %w", err)
	}
	logdogPrefixSN, err := logdog_types.MakeStreamName("", "led", uid, hex.EncodeToString(buf))
	if err != nil {
		return errors.Fmt("generating logdog token: %w", err)
	}
	logdogPrefix := string(logdogPrefixSN)
	logdogProjectPrefix := path.Join(bb.BbagentArgs.Build.Infra.Logdog.Project, logdogPrefix)

	// TODO(iannucci): change logdog project to something reserved to 'led' tasks.
	// Though if we merge logdog into resultdb, this hopefully becomes moot.
	bb.BbagentArgs.Build.Infra.Logdog.Prefix = logdogPrefix

	// Pass the CIPD package or isolate containing the recipes code into
	// the led recipe module. This gives the build the information it needs
	// to launch child builds using the same version of the recipes code.
	//
	// The logdog prefix is unique to each led job, so it can be used as an
	// ID for the job.
	props := ledProperties{LedRunID: logdogProjectPrefix}
	casUserPayload, err := jd.Info().CurrentIsolated()
	if err != nil {
		return errors.Fmt("failed to get CAS user payload for the build: %w", err)
	}
	if exe := bb.GetBbagentArgs().GetBuild().GetExe(); exe.GetCipdPackage() != "" {
		props.CIPDInput = &cipdInput{
			Package: exe.CipdPackage,
			Version: exe.CipdVersion,
		}
	} else if casUserPayload.GetDigest() != nil {
		props.RbeCasInput = proto.Clone(casUserPayload).(*swarmingpb.CASReference)
	}

	// in case both isolate and rbe-cas properties are set in "$recipe_engine/led".
	bb.WriteProperties(map[string]any{
		"$recipe_engine/led": nil,
	})
	bb.WriteProperties(map[string]any{
		"$recipe_engine/led": props,
	})

	streamName := "build.proto"
	if bb.LegacyKitchen {
		streamName = "annotations"
	}

	logdogHost := "logs.chromium.org"
	if strings.Contains(jd.Info().SwarmingHostname(), "-dev") {
		logdogHost = "luci-logdog-dev.appspot.com"
	}

	logdogTag := "log_location:logdog://" + path.Join(
		logdogHost, logdogProjectPrefix, "+", streamName)

	return jd.Edit(func(je Editor) {
		je.Tags([]string{logdogTag, "allow_milo:1"})
	})
}

type expiringDims struct {
	absolute time.Duration // from scheduling task
	relative time.Duration // from previous slice

	// key -> values
	dimensions map[string]stringset.Set
}

func (ed *expiringDims) addDimVals(key string, values ...string) {
	if ed.dimensions == nil {
		ed.dimensions = map[string]stringset.Set{}
	}
	if set, ok := ed.dimensions[key]; !ok {
		ed.dimensions[key] = stringset.NewFromSlice(values...)
	} else {
		set.AddAll(values)
	}
}

func (ed *expiringDims) updateFrom(other *expiringDims) {
	for key, values := range other.dimensions {
		ed.addDimVals(key, values.ToSlice()...)
	}
}

func (ed *expiringDims) createWith(template *swarmingpb.TaskProperties) *swarmingpb.TaskProperties {
	if len(template.Dimensions) != 0 {
		panic("impossible; createWith called with dimensions already set")
	}

	ret := proto.Clone(template).(*swarmingpb.TaskProperties)

	newDims := make([]*swarmingpb.StringPair, 0, len(ed.dimensions))
	for _, key := range keysOf(ed.dimensions) {
		for _, value := range ed.dimensions[key].ToSortedSlice() {
			newDims = append(newDims, &swarmingpb.StringPair{
				Key: key, Value: value})
		}
	}
	ret.Dimensions = newDims

	return ret
}

func (jd *Definition) makeExpiringSliceData() (ret []*expiringDims, err error) {
	bb := jd.GetBuildbucket()
	expirationSet := map[time.Duration]*expiringDims{}
	nonExpiring := &expiringDims{}
	getExpiringSlot := func(dimType, name string, protoDuration *durpb.Duration) (*expiringDims, error) {
		var dur time.Duration
		if protoDuration != nil {
			var err error
			if dur, err = ptypes.Duration(protoDuration); err != nil {
				return nil, errors.Fmt("parsing %s %q expiration: %w", dimType, name, err)
			}
		}
		if dur > 0 {
			data, ok := expirationSet[dur]
			if !ok {
				data = &expiringDims{absolute: dur}
				expirationSet[dur] = data
			}
			return data, nil
		}
		return nil, nil
	}
	// Cache and dimension expiration have opposite defaults for 0 or negative
	// times.
	//
	// Cache entries with WaitForWarmCache <= 0 mean that the dimension for the
	// cache essentially expires at 0.
	//
	// Dimension entries with Expiration <= 0 mean that the dimension expires at
	// 'infinity'
	for _, cache := range bb.BbagentArgs.GetBuild().GetInfra().GetSwarming().GetCaches() {
		slot, err := getExpiringSlot("cache", cache.Name, cache.WaitForWarmCache)
		if err != nil {
			return nil, err
		}
		if slot != nil {
			slot.addDimVals("caches", cache.Name)
		}
	}
	for _, dim := range bb.BbagentArgs.GetBuild().GetInfra().GetSwarming().GetTaskDimensions() {
		slot, err := getExpiringSlot("dimension", dim.Key, dim.Expiration)
		if err != nil {
			return nil, err
		}
		if slot == nil {
			slot = nonExpiring
		}
		slot.addDimVals(dim.Key, dim.Value)
	}

	ret = make([]*expiringDims, 0, len(expirationSet))
	if len(expirationSet) > 0 {
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
	}
	if total, err := ptypes.Duration(bb.BbagentArgs.Build.SchedulingTimeout); err == nil {
		if len(ret) == 0 || ret[len(ret)-1].absolute < total {
			// if the task's total expiration time is greater than the last slice's
			// expiration, then use nonExpiring as the last slice.
			nonExpiring.absolute = total
			if len(ret) > 0 {
				nonExpiring.relative = total - ret[len(ret)-1].absolute
			} else {
				nonExpiring.relative = total
			}
			ret = append(ret, nonExpiring)
		} else {
			// otherwise, add all of nonExpiring's guts to the last slice.
			ret[len(ret)-1].updateFrom(nonExpiring)
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
		ret[i].updateFrom(ret[i+1])
	}

	return
}

func (jd *Definition) generateCommand(ctx context.Context, ks KitchenSupport) ([]string, error) {
	bb := jd.GetBuildbucket()

	if bb.LegacyKitchen {
		return ks.GenerateCommand(ctx, bb)
	}

	ret := []string{"bbagent${EXECUTABLE_SUFFIX}"}
	if bb.FinalBuildProtoPath != "" {
		ret = append(ret, "--output", path.Join("${ISOLATED_OUTDIR}", bb.FinalBuildProtoPath))
	}
	bb.BbagentArgs.Build.Infra.Buildbucket.Hostname = ""
	if bb.BbagentArgs.CacheDir == "" {
		bb.BbagentArgs.CacheDir = bb.BbagentArgs.Build.GetInfra().GetBbagent().GetCacheDir()
	}
	if bb.BbagentArgs.PayloadPath == "" {
		bb.BbagentArgs.PayloadPath = "kitchen-checkout"
	}
	return append(ret, bbinput.Encode(bb.BbagentArgs)), nil
}

func (jd *Definition) generateCIPDPackages() (cipdPackages []*swarmingpb.CipdPackage) {
	cipdPackages = ([]*swarmingpb.CipdPackage)(nil)
	bb := jd.GetBuildbucket()
	if !bb.BbagentDownloadCIPDPkgs() {
		cipdPackages = append(cipdPackages, bb.CipdPackages...)
		return
	}

	if agentSrc := bb.BbagentArgs.GetBuild().GetInfra().GetBuildbucket().GetAgent().GetSource(); agentSrc != nil {
		if cipdSource := agentSrc.GetCipd(); cipdSource != nil {
			cipdPackages = append(cipdPackages, &swarmingpb.CipdPackage{
				Path:        ".",
				PackageName: cipdSource.Package,
				Version:     cipdSource.Version,
			})
		}
	}
	return
}

// FlattenToSwarming modifies this Definition to populate the Swarming field
// from the Buildbucket field.
//
// After flattening, HighLevelEdit functionality will no longer work on this
// Definition.
//
// `uid` and `parentTaskId`, if specified, override the user and parentTaskId
// fields, respectively.
func (jd *Definition) FlattenToSwarming(ctx context.Context, uid, parentTaskId string, ks KitchenSupport, resultdb RDBEnablement) error {
	if sw := jd.GetSwarming(); sw != nil {
		if uid != "" {
			sw.Task.User = uid
		}
		if parentTaskId != "" {
			sw.Task.ParentTaskId = parentTaskId
		}
		switch resultdb {
		case RDBOff:
			sw.Task.Resultdb = nil
		case RDBOn:
			if sw.Task.Realm != "" {
				sw.Task.Resultdb = &swarmingpb.ResultDBCfg{
					Enable: true,
				}
			} else {
				return errors.New("ResultDB cannot be enabled on raw swarming tasks if the realm field is unset")
			}
		default:
		}
		return nil
	}
	err := jd.addLedProperties(ctx, uid)
	if err != nil {
		return errors.Fmt("adding led properties: %w", err)
	}

	expiringDims, err := jd.makeExpiringSliceData()
	if err != nil {
		return errors.Fmt("calculating expirations: %w", err)
	}

	bb := jd.GetBuildbucket()
	bbi := bb.GetBbagentArgs().GetBuild().GetInfra()
	project := bb.GetBbagentArgs().GetBuild().GetBuilder().GetProject()
	bucket := bb.GetBbagentArgs().GetBuild().GetBuilder().GetBucket()
	if project == "" || bucket == "" {
		return errors.New("incomplete Builder ID, need both `project` and `bucket` set")
	}
	sw := &Swarming{
		Hostname: jd.Info().SwarmingHostname(),
		Task: &swarmingpb.NewTaskRequest{
			Name:           jd.Info().TaskName(),
			Realm:          fmt.Sprintf("%s:%s", project, bucket),
			ParentTaskId:   parentTaskId,
			Priority:       jd.Info().Priority(),
			ServiceAccount: bbi.GetSwarming().GetTaskServiceAccount(),
			Tags:           jd.Info().Tags(),
			User:           uid,
			TaskSlices:     make([]*swarmingpb.TaskSlice, len(expiringDims)),
		},
	}

	// Enable swarming/resultdb integration.
	enableRDB := (resultdb == RDBOn || (resultdb == "" && bbi.GetResultdb().GetInvocation() != ""))
	if enableRDB {
		// Clear the original build's ResultDB invocation.
		bbi.Resultdb.Invocation = ""
		sw.Task.Resultdb = &swarmingpb.ResultDBCfg{
			Enable: true,
		}
	}

	var casUserPayload *swarmingpb.CASReference
	// Do not set CAS input to task slices if bbagent handles downloading packages.
	if !bb.BbagentDownloadCIPDPkgs() {
		casUserPayload, err = jd.Info().CurrentIsolated()
		if err != nil {
			return errors.Fmt("failed to get CAS user payload for the build: %w", err)
		}
	}
	baseProperties := &swarmingpb.TaskProperties{
		CipdInput: &swarmingpb.CipdInput{
			Packages: jd.generateCIPDPackages(),
		},
		CasInputRoot: casUserPayload,

		EnvPrefixes:          bb.EnvPrefixes,
		ExecutionTimeoutSecs: int32(bb.BbagentArgs.Build.ExecutionTimeout.GetSeconds()),

		// TODO(iannucci): When build creation is done in Go, share this 3 minute
		// constant between here and there.  Or, better, implement CreateBuild so we
		// don't have to do this at all.
		GracePeriodSecs: int32(bb.BbagentArgs.Build.GracePeriod.GetSeconds()) + 180,
	}

	if bb.Containment.GetContainmentType() != swarmingpb.Containment_NOT_SPECIFIED {
		baseProperties.Containment = bb.Containment
	}

	baseProperties.Env = make([]*swarmingpb.StringPair, len(bb.EnvVars)+1)
	copy(baseProperties.Env, bb.EnvVars)
	expEnvValue := "FALSE"
	if bb.BbagentArgs.Build.Input.Experimental {
		expEnvValue = "TRUE"
	}
	baseProperties.Env[len(baseProperties.Env)-1] = &swarmingpb.StringPair{
		Key:   "BUILDBUCKET_EXPERIMENTAL",
		Value: expEnvValue,
	}

	if caches := bb.BbagentArgs.Build.Infra.Swarming.GetCaches(); len(caches) > 0 {
		baseProperties.Caches = make([]*swarmingpb.CacheEntry, len(caches))
		for i, cache := range caches {
			baseProperties.Caches[i] = &swarmingpb.CacheEntry{
				Name: cache.Name,
				Path: path.Join(bb.CacheDir(), cache.Path),
			}
		}
	}

	baseProperties.Command, err = jd.generateCommand(ctx, ks)
	if err != nil {
		return errors.Fmt("generating Command: %w", err)
	}

	if exe := bb.BbagentArgs.Build.Exe; exe.GetCipdPackage() != "" && !bb.BbagentDownloadCIPDPkgs() {
		baseProperties.CipdInput.Packages = append(baseProperties.CipdInput.Packages, &swarmingpb.CipdPackage{
			PackageName: exe.CipdPackage,
			Version:     exe.CipdVersion,
			Path:        bb.PayloadPath(),
		})
	}

	for i, dat := range expiringDims {
		sw.Task.TaskSlices[i] = &swarmingpb.TaskSlice{
			ExpirationSecs: int32(dat.relative.Seconds()),
			Properties:     dat.createWith(baseProperties),
		}
	}

	if err := experiments.Apply(ctx, bb.BbagentArgs.Build, sw.Task); err != nil {
		return errors.Fmt("applying experiments: %w", err)
	}

	jd.JobType = &Definition_Swarming{Swarming: sw}
	return nil
}
