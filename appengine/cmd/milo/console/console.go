// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package console

import (
	"fmt"
	"strings"

	"github.com/luci/luci-go/appengine/cmd/milo/buildbot"
	"github.com/luci/luci-go/appengine/cmd/milo/git"
	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"github.com/luci/luci-go/common/clock"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// Returns results of build[commit_index][builder_index]
func GetConsoleBuilds(
	c context.Context, module string,
	builders []resp.BuilderRef, commits []string) (
	[][]*resp.ConsoleBuild, error) {

	switch module {
	case "buildbot":
		return buildbot.GetConsoleBuilds(c, builders, commits)
	// The case for buildbucket and dm goes here.
	default:
		panic(fmt.Errorf("Unrecognized module %s", module))
	}
}

func console(c context.Context, def *ConsoleDef) (*resp.Console, error) {
	tStart := clock.Now(c)
	// Lookup Commits.  For this hack, we're just gonna hardcode src.git
	commits, err := git.GetCommits(c, def.Repository, def.Branch, 25)
	if err != nil {
		return nil, err
	}
	tGitiles := clock.Now(c)
	log.Debugf(c, "Loading commits took %s.", tGitiles.Sub(tStart))
	commitNames := make([]string, len(commits))
	for i, commit := range commits {
		commitNames[i] = commit.Revision
	}

	// HACK(hinoka): This only supports buildbot....
	builders := make([]resp.BuilderRef, len(def.Builders))
	for i, b := range def.Builders {
		builders[i] = resp.BuilderRef{
			b.Module, b.Name, strings.Split(b.Category, "|"), b.ShortName,
		}
	}
	cb, err := GetConsoleBuilds(c, "buildbot", builders, commitNames)
	tConsole := clock.Now(c)
	log.Debugf(c, "Loading the console took a total of %s.", tConsole.Sub(tGitiles))
	if err != nil {
		return nil, err
	}
	ccb := make([]resp.CommitBuild, len(commits))
	for i, commit := range commitNames {
		// TODO(hinoka): Not like this
		ccb[i].Commit = resp.Commit{Revision: commit}
		ccb[i].Build = cb[i]
	}

	cs := &resp.Console{
		Name:       def.Name,
		Commit:     ccb,
		BuilderRef: builders,
	}

	return cs, nil
}
