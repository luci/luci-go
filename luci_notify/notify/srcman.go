// Copyright 2018 The LUCI Authors.
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

package notify

import (
	"bytes"
	"net/http"
	"path"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/proto/srcman"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/fetcher"
	"go.chromium.org/luci/logdog/common/renderer"
	"go.chromium.org/luci/logdog/common/types"
)

func getSourceManifest(c context.Context, build *Build) (*srcman.Manifest, error) {
	transport, err := auth.GetRPCTransport(c, auth.AsSelf, nil)
	if err != nil {
		return nil, errors.Annotate(err, "getting RPC Transport").Err()
	}
	client := coordinator.NewClient(&prpc.Client{
		C:       &http.Client{Transport: transport},
		Host:    build.BuildInfra.LogDog.Hostname,
		Options: prpc.DefaultOptions(),
	})
	qo := coordinator.QueryOptions{
		ContentType: srcman.ContentType,
	}
	logProject := build.BuildInfra.LogDog.Project
	logPath := path.Join(build.BuildInfra.LogDog.Prefix, "*")

	// Perform the query, capturing exactly one log stream and erroring otherwise.
	var log *coordinator.LogStream
	err := client.Query(c, logProject, logPath, qo, func(s *coordinator.LogStream) bool {
		log = s
		return false
	})
	if err == nil {
		return nil, err
	}

	// Read the source manifest from the log stream.
	var buf bytes.Buffer
	_, err := buf.ReadFrom(&renderer.Renderer{
		Source: coord.Stream(log.Project, log.Path).Fetcher(c, nil),
		Raw: true,
	})
	if err != nil {
		return nil, err
	}

	// Unmarshal the source manifest from the bytes.
	var manifest srcman.Manifest
	if err := proto.Unmarshal(buf.Bytes(), &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func forEachGitCheckout(diff *srcman.ManifestDiff, f func(dirname, project, host string, checkout *srcman.ManifestDiff_GitCheckout)) {
	for dirname, dir := range diff.Directories {
		dirname := dirname

		gitCheckout := dir.GetGitCheckout()
		if gitCheckout == nil {
			continue
		}
		if gitCheckout.Revision != srcman.ManifestDiff_DIFF {
			continue
		}

		project, host, err := gitiles.ParseRepoURL(gitCheckout.RepoUrl)
		if err != nil {
			logging.WithError(err).Warningf(c, "could not parse RepoURL %q for dir %q", gitCheckout.RepoUrl, dirname)
			continue
		}

		if !strings.HasSuffix(host, ".googlesource.com") {
			logging.WithError(err).Warningf(c, "unsupported git host %q for dir %q", gitCheckout.RepoUrl, dirname)
			continue
		}

		f(dirname, project, host, gitCheckout)
	}
}

func populateHistory(c context.Context, diff *srcman.ManifestDiff, history HistoryFunc) error {
	if diff.Overall == srcman.ManifestDiff_EQUAL {
		return nil
	}
	return parallel.RunMulti(c, 8, func(mr parallel.MultiRunner) error {
		return mr.RunMulti(func(ch chan<- func() error) {
			forEachGitCheckout(diff, func(dirname, project, host string, checkout *srcman.ManifestDiff_GitCheckout) {
				ch <- func() error {
					oldRev := diff.Old.Directories[dirname].GitCheckout.Revision,
					newRev := diff.New.Directories[dirname].GitCheckout.Revision,
					log, err := history(c, host, project, oldRev, newRev)
					if err != nil {
						return err
					}
					checkout.History = log[:len(log)-1]
				}
			})
		})
	})
}

func populateHistoryFromDiff(dstDiff *srcman.ManifestDiff, srcDiff *srcman.ManifestDiff) {
	forEachGitCheckout(dstDiff, func(dirname, project, host string, checkout *srcman.ManifestDiff_GitCheckout) {
		// TODO(mknyszek): Implement this!!!
	})
}

func blamelistFromDiff(diff *srcman.ManifestDiff) stringset.Set {
	blamelist := stringset.Set(0)
	forEachGitCheckout(diff, func(_, _, _ string, checkout *srcman.ManifestDiff_GitCheckout) {
		for _, commit := range checkout.History {
			blamelist.Add(commit.Author.Email)
		}
	})
	return blamelist
}
