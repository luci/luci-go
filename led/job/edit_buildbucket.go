// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
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
	panic("implement me")
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
	panic("implement me")
}

func (bbm *buildbucketEditor) Priority(priority int32) {
	panic("implement me")
}

func (bbm *buildbucketEditor) Properties(props map[string]string, auto bool) {
	panic("implement me")
}

func (bbm *buildbucketEditor) SwarmingHostname(host string) {
	panic("implement me")
}

func (bbm *buildbucketEditor) Experimental(isExperimental bool) {
	panic("implement me")
}

func (bbm *buildbucketEditor) PrefixPathEnv(values []string) {
	panic("implement me")
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
