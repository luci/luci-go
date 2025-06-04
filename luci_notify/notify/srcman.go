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
	"context"
	"net/http"
	"path"
	"strings"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/srcman"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/renderer"
	"go.chromium.org/luci/server/auth"
)

// CheckoutFunc is a function that given a Build, produces a source checkout
// related to that build.
type CheckoutFunc func(context.Context, *Build) (Checkout, error)

// srcmanCheckout is a CheckoutFunc which retrieves a source checkout related
// to a build by querying LogDog for a source manifest stream associated with
// that build. It assumes that the build has exactly one source manifest.
func srcmanCheckout(c context.Context, build *Build) (Checkout, error) {
	if build.Infra == nil || build.Infra.Logdog == nil || build.Infra.Logdog.Hostname == "" {
		return nil, errors.New("logdog hostname is not set in the build proto")
	}
	transport, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		return nil, errors.Fmt("getting RPC Transport: %w", err)
	}
	client := coordinator.NewClient(&prpc.Client{
		C:       &http.Client{Transport: transport},
		Host:    build.Infra.Logdog.Hostname,
		Options: prpc.DefaultOptions(),
	})
	qo := coordinator.QueryOptions{
		ContentType: srcman.ContentTypeSourceManifest,
	}
	logProject := build.Infra.Logdog.Project
	logPath := path.Join(build.Infra.Logdog.Prefix, "+", "**")

	// Perform the query, capturing exactly one log stream and erroring otherwise.
	var log *coordinator.LogStream
	err = client.Query(c, logProject, logPath, qo, func(s *coordinator.LogStream) bool {
		log = s
		return false
	})
	switch {
	case err != nil:
		return nil, grpcutil.WrapIfTransient(err)
	case log == nil:
		logging.Infof(c, "unable to find source manifest in project %s at path %s",
			build.Infra.Logdog.Project, logPath)
		return nil, nil
	}

	// Read the source manifest from the log stream.
	var buf bytes.Buffer
	_, err = buf.ReadFrom(&renderer.Renderer{
		Source: client.Stream(logProject, log.Path).Fetcher(c, nil),
		Raw:    true,
	})
	if err != nil {
		return nil, transient.Tag.Apply(errors.
			Fmt("failed to read stream: %w", err))
	}

	// Unmarshal the source manifest from the bytes.
	var manifest srcman.Manifest
	if err := proto.Unmarshal(buf.Bytes(), &manifest); err != nil {
		return nil, err
	}

	results := make(Checkout)
	for dirname, dir := range manifest.Directories {
		gitCheckout := dir.GetGitCheckout()
		if gitCheckout == nil {
			continue
		}

		url, err := gitiles.NormalizeRepoURL(gitCheckout.RepoUrl, false)
		if err != nil {
			logging.WithError(err).Warningf(c, "could not parse RepoURL %q for dir %q", gitCheckout.RepoUrl, dirname)
			continue
		}

		if !strings.HasSuffix(url.Host, ".googlesource.com") {
			logging.WithError(err).Warningf(c, "unsupported git host %q for dir %q", gitCheckout.RepoUrl, dirname)
			continue
		}
		results[url.String()] = gitCheckout.Revision
	}
	return results, nil
}
