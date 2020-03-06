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
	"encoding/json"
	"path"
	"sort"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	api "go.chromium.org/luci/swarming/proto/api"
)

// buildbucketEditor is a temporary type returned by
// Definition.Edit. It holds a mutable buildbucket-based
// Definition and an error, allowing a series of Edit commands to be called
// while buffering the error (if any).  Obtain the modified Definition (or
// error) by calling Finalize.
type buildbucketEditor struct {
	jd          *Definition
	bb          *Buildbucket
	userPayload *api.CASTree

	err error
}

var _ HighLevelEditor = (*buildbucketEditor)(nil)

func newBuildbucketEditor(jd *Definition) *buildbucketEditor {
	bb := jd.GetBuildbucket()
	if bb == nil {
		panic(errors.New("impossible: only supported for Buildbucket builds"))
	}
	bb.EnsureBasics()

	if jd.UserPayload == nil {
		jd.UserPayload = &api.CASTree{}
	}
	return &buildbucketEditor{jd, bb, jd.UserPayload, nil}
}

func (bbm *buildbucketEditor) Close() error {
	return bbm.err
}

func (bbm *buildbucketEditor) tweak(fn func() error) {
	if bbm.err == nil {
		bbm.err = fn()
	}
}

func (bbm *buildbucketEditor) Tags(values []string) {
	if len(values) == 0 {
		return
	}

	bbm.tweak(func() (err error) {
		if err = validateTags(values); err == nil {
			bbm.bb.ExtraTags = append(bbm.bb.ExtraTags, values...)
			sort.Strings(bbm.bb.ExtraTags)
		}
		return nil
	})
}

func (bbm *buildbucketEditor) TaskPayload(cipdPkg, cipdVers, dirInTask string) {
	bbm.tweak(func() error {
		bbm.bb.BbagentArgs.ExecutablePath = path.Join(dirInTask, "luciexe")
		if cipdPkg != "" && cipdVers != "" {
			bbm.bb.BbagentArgs.Build.Exe = &bbpb.Executable{
				CipdPackage: cipdPkg,
				CipdVersion: cipdVers,
			}
		} else if cipdPkg == "" && cipdVers == "" {
			bbm.bb.BbagentArgs.Build.Exe = nil
		} else {
			return errors.Reason(
				"cipdPkg and cipdVers must both be set or both be empty: cipdPkg=%q cipdVers=%q",
				cipdPkg, cipdVers).Err()
		}
		return nil
	})
}

func (bbm *buildbucketEditor) ClearCurrentIsolated() {
	bbm.tweak(func() error {
		bbm.userPayload.Digest = ""
		return nil
	})
}

func (bbm *buildbucketEditor) ClearDimensions() {
	bbm.tweak(func() error {
		bbm.bb.BbagentArgs.Build.Infra.Swarming.TaskDimensions = nil
		return nil
	})
}

func (bbm *buildbucketEditor) SetDimensions(dims ExpiringDimensions) {
	bbm.ClearDimensions()
	dec := DimensionEditCommands{}
	for key, vals := range dims {
		dec[key] = &DimensionEditCommand{SetValues: vals}
	}
	bbm.EditDimensions(dec)
}

func (bbm *buildbucketEditor) EditDimensions(dimEdits DimensionEditCommands) {
	if len(dimEdits) == 0 {
		return
	}

	bbm.tweak(func() error {
		dims, err := bbm.jd.Info().Dimensions()
		if err != nil {
			return err
		}

		dimMap := dims.toLogical()
		dimEdits.apply(dimMap, 0)

		sw := bbm.bb.BbagentArgs.Build.Infra.Swarming
		var maxExp time.Duration
		newDims := make([]*bbpb.RequestedDimension, 0,
			len(sw.TaskDimensions)+len(dimEdits))
		for _, key := range keysOf(dimMap) {
			valueExp := dimMap[key]
			for _, value := range keysOf(valueExp) {
				exp := valueExp[value]
				if exp > maxExp {
					maxExp = exp
				}

				toAdd := &bbpb.RequestedDimension{
					Key:   key,
					Value: value,
				}
				if exp > 0 {
					toAdd.Expiration = ptypes.DurationProto(exp)
				}
				newDims = append(newDims, toAdd)
			}
		}
		sw.TaskDimensions = newDims

		build := bbm.bb.BbagentArgs.Build
		var curTimeout time.Duration
		if build.SchedulingTimeout != nil {
			var err error
			if curTimeout, err = ptypes.Duration(build.SchedulingTimeout); err != nil {
				return err
			}
		}
		if maxExp > curTimeout {
			build.SchedulingTimeout = ptypes.DurationProto(maxExp)
		}
		return nil
	})
}

func (bbm *buildbucketEditor) Env(env map[string]string) {
	if len(env) == 0 {
		return
	}

	bbm.tweak(func() error {
		updateStringPairList(&bbm.bb.EnvVars, env)
		return nil
	})
}

func (bbm *buildbucketEditor) Priority(priority int32) {
	if priority < 0 {
		return
	}
	bbm.tweak(func() error {
		bbm.bb.BbagentArgs.Build.Infra.Swarming.Priority = priority
		return nil
	})
}

func (bbm *buildbucketEditor) Properties(props map[string]string, auto bool) {
	if len(props) == 0 {
		return
	}
	bbm.tweak(func() error {
		toWrite := map[string]interface{}{}

		for k, v := range props {
			if v == "" {
				toWrite[k] = nil
			} else {
				var obj interface{}
				if err := json.Unmarshal([]byte(v), &obj); err != nil {
					if !auto {
						return err
					}
					obj = v
				}
				toWrite[k] = obj
			}
		}

		bbm.bb.WriteProperties(toWrite)
		return nil
	})
}

func (bbm *buildbucketEditor) CIPDPkgs(cipdPkgs CIPDPkgs) {
	bbm.tweak(func() error {
		cipdPkgs.updateCipdPkgs(&bbm.bb.CipdPackages)
		return nil
	})
}

func (bbm *buildbucketEditor) SwarmingHostname(host string) {
	if host == "" {
		return
	}

	bbm.tweak(func() (err error) {
		bbm.bb.BbagentArgs.Build.Infra.Swarming.Hostname = host
		return
	})
}

func (bbm *buildbucketEditor) Experimental(isExperimental bool) {
	bbm.tweak(func() error {
		bbm.bb.BbagentArgs.Build.Input.Experimental = isExperimental
		return nil
	})
}

func (bbm *buildbucketEditor) PrefixPathEnv(values []string) {
	if len(values) == 0 {
		return
	}

	bbm.tweak(func() error {
		updatePrefixPathEnv(values, &bbm.bb.EnvPrefixes)
		return nil
	})
}

func (bbm *buildbucketEditor) AddGerritChange(cl *bbpb.GerritChange) {
	if cl == nil {
		return
	}

	bbm.tweak(func() error {
		gc := &bbm.bb.BbagentArgs.Build.Input.GerritChanges
		for _, change := range *gc {
			if proto.Equal(change, cl) {
				return nil
			}
		}
		*gc = append(*gc, cl)
		return nil
	})
}

func (bbm *buildbucketEditor) RemoveGerritChange(cl *bbpb.GerritChange) {
	if cl == nil {
		return
	}

	bbm.tweak(func() error {
		gc := &bbm.bb.BbagentArgs.Build.Input.GerritChanges
		for idx, change := range *gc {
			if proto.Equal(change, cl) {
				*gc = append((*gc)[:idx], (*gc)[idx+1:]...)
				return nil
			}
		}
		return nil
	})
}

func (bbm *buildbucketEditor) GitilesCommit(commit *bbpb.GitilesCommit) {
	bbm.tweak(func() error {
		bbm.bb.BbagentArgs.Build.Input.GitilesCommit = commit
		return nil
	})
}
