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
	panic("implement me")
}

func (bbm *buildbucketEditor) ClearCurrentIsolated() {
	bbm.tweak(func() error {
		bbm.userPayload.Digest = ""
		return nil
	})
}

func (bbm *buildbucketEditor) ClearDimensions() {
	panic("implement me")
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
	panic("implement me")
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
	panic("implement me")
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
	panic("implement me")
}

func (bbm *buildbucketEditor) RemoveGerritChange(cl *bbpb.GerritChange) {
	panic("implement me")
}

func (bbm *buildbucketEditor) GitilesCommit(commit *bbpb.GitilesCommit) {
	panic("implement me")
}
