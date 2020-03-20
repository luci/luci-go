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

func (bbe *buildbucketEditor) Close() error {
	return bbe.err
}

func (bbe *buildbucketEditor) tweak(fn func() error) {
	if bbe.err == nil {
		bbe.err = fn()
	}
}

func (bbe *buildbucketEditor) Tags(values []string) {
	if len(values) == 0 {
		return
	}

	bbe.tweak(func() (err error) {
		if err = validateTags(values); err == nil {
			bbe.bb.ExtraTags = append(bbe.bb.ExtraTags, values...)
			sort.Strings(bbe.bb.ExtraTags)
		}
		return nil
	})
}

func (bbe *buildbucketEditor) TaskPayload(cipdPkg, cipdVers, dirInTask string) {
	panic("implement me")
}

func (bbe *buildbucketEditor) ClearCurrentIsolated() {
	bbe.tweak(func() error {
		bbe.userPayload.Digest = ""
		return nil
	})
}

func (bbe *buildbucketEditor) ClearDimensions() {
	panic("implement me")
}

func (bbe *buildbucketEditor) Env(env map[string]string) {
	if len(env) == 0 {
		return
	}

	bbe.tweak(func() error {
		updateStringPairList(&bbe.bb.EnvVars, env)
		return nil
	})
}

func (bbe *buildbucketEditor) Priority(priority int32) {
	panic("implement me")
}

func (bbe *buildbucketEditor) Properties(props map[string]string, auto bool) {
	panic("implement me")
}

func (bbe *buildbucketEditor) SwarmingHostname(host string) {
	panic("implement me")
}

func (bbe *buildbucketEditor) Experimental(isExperimental bool) {
	panic("implement me")
}

func (bbe *buildbucketEditor) PrefixPathEnv(values []string) {
	if len(values) == 0 {
		return
	}

	bbe.tweak(func() error {
		updatePrefixPathEnv(values, &bbe.bb.EnvPrefixes)
		return nil
	})
}

func (bbe *buildbucketEditor) AddGerritChange(cl *bbpb.GerritChange) {
	panic("implement me")
}

func (bbe *buildbucketEditor) RemoveGerritChange(cl *bbpb.GerritChange) {
	panic("implement me")
}

func (bbe *buildbucketEditor) GitilesCommit(commit *bbpb.GitilesCommit) {
	panic("implement me")
}
