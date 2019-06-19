// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildsource

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/frontend/ui"
)

// BuilderID is the universal ID of a builder, and has the form:
//   buildbucket/bucket/builder
//   buildbot/master/builder
type BuilderID string

// Split breaks the BuilderID into pieces.
//   - backend is either 'buildbot' or 'buildbucket'
//   - backendGroup is either the bucket or master name
//   - builderName is the builder name.
//
// Returns an error if the BuilderID is malformed (wrong # slashes) or if any of
// the pieces are empty.
func (b BuilderID) Split() (backend, backendGroup, builderName string, err error) {
	toks := strings.SplitN(string(b), "/", 3)
	if len(toks) != 3 {
		err = errors.Reason("bad BuilderID: not enough tokens: %q", b).
			Tag(grpcutil.InvalidArgumentTag).Err()
		return
	}
	backend, backendGroup, builderName = toks[0], toks[1], toks[2]
	switch backend {
	case "buildbot", "buildbucket":
	default:
		err = errors.Reason("bad BuilderID: unknown backend %q", backend).
			Tag(grpcutil.InvalidArgumentTag).Err()
		return
	}
	if backendGroup == "" {
		err = errors.New("bad BuilderID: empty backendGroup", grpcutil.InvalidArgumentTag)
		return
	}
	if builderName == "" {
		err = errors.New("bad BuilderID: empty builderName", grpcutil.InvalidArgumentTag)
		return
	}
	return
}

// Get allows you to obtain the resp.Builder that corresponds with this
// BuilderID.
func (b BuilderID) Get(c context.Context, limit int, cursor string) (*ui.BuilderLegacy, error) {
	// TODO(iannucci): replace these implementations with a BuildSummary query.
	source, group, builderName, err := b.Split()
	if err != nil {
		return nil, err
	}

	var builder *ui.BuilderLegacy
	var consoles []*common.Console
	err = parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() (err error) {
			switch source {
			case "buildbot":
				builder, err = buildbot.GetBuilder(c, group, builderName, limit, cursor)
			case "buildbucket":
				bid := buildbucket.NewBuilderID(group, builderName)
				builder, err = buildbucket.GetBuilder(c, bid, limit, cursor)
			default:
				panic(fmt.Errorf("unexpected build source %q", source))
			}
			return
		}
		work <- func() (err error) {
			consoles, err = common.GetAllConsoles(c, string(b))
			return
		}
	})
	if err != nil {
		return nil, err
	}

	builder.Groups = make([]*ui.Link, len(consoles))
	for i, c := range consoles {
		builder.Groups[i] = ui.NewLink(
			fmt.Sprintf("%s / %s", c.ProjectID(), c.ID),
			fmt.Sprintf("/p/%s/g/%s", c.ProjectID(), c.ID),
			fmt.Sprintf("builder group %s in project %s", c.ID, c.ProjectID()))
	}
	sort.Slice(builder.Groups, func(i, j int) bool {
		return builder.Groups[i].Label < builder.Groups[j].Label
	})

	for _, b := range builder.FinishedBuilds {
		if len(b.Blame) > 0 {
			builder.HasBlamelist = true
			break
		}
	}

	return builder, nil
}

// SelfLink returns LUCI URL of the builder.
func (b BuilderID) SelfLink(project string) string {
	return model.BuilderIDLink(string(b), project)
}

// Buildbot returns true iff this BuilderID originates from a buildbot builder.
func (b BuilderID) Buildbot() bool {
	return strings.HasPrefix(string(b), "buildbot/")
}
