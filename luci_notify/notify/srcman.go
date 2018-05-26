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

func deriveBlamelist(c context.Context, diff *srcman.ManifestDiff, history HistoryFunc) (stringset.Set, error) {
	blamelists := make([]stringset.Set, len(diff.Directories))
	err := parallel.RunMulti(c, 8, func(mr parallel.MultiRunner) error {
		return mr.RunMulti(func(ch chan<- func() error) {
			blamelistIndex := 0
			for dirname, dir := range diff.Directories {
				i := blamelistIndex
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

				ch <- func() error {
					oldRev := diff.Old.Directories[dirname].GitCheckout.Revision,
					newRev := diff.New.Directories[dirname].GitCheckout.Revision,
					log, err := history(c, host, project, oldRev, newRev)
					if err != nil {
						return err
					}
					emails := stringset.New(len(log))
					for _, commit := range log {
						emails.Add(commit.Author.Email)
					}
					blamelists[i] = emails
				}
				blamelistIndex += 1
			}
		})
	})
	if err != nil {
		return nil, err
	}
	blamelist := stringset.New(0)
	for _, emails := range blamelists {
		blamelist.Union(emails)
	}
	return blamelist, nil
}
