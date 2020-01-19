// Copyright 2019 The LUCI Authors.
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

package util

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// IsolatedFilesToString returns a string describing isolated files so we can track those that need
// additional handling.
func IsolatedFilesToString(fMap map[string]*pb.Artifact) string {
	msg := make([]string, 0, len(fMap))
	for name, art := range fMap {
		msg = append(msg, fmt.Sprintf("%s %s", name, art.FetchUrl))
	}
	return strings.Join(msg, "\n")
}

// IsolatedFileToArtifact returns a possibly partial pb.Artifact representing the isolated.File.
func IsolatedFileToArtifact(isolateServer, ns, relPath string, f *isolated.File) *pb.Artifact {
	// We don't know how to handle symlink files, so return nil for the caller to deal with it.
	if f.Link != nil {
		return nil
	}

	host := isolateServer
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")

	// Otherwise, populate the artifact fields.
	a := &pb.Artifact{
		Name: NormalizeIsolatedPath(relPath),
		// FetchURL is supposed to be an https:// URL, but we use an isolate://
		// URL in storage, and convert it to a https:// URL before returning to the
		// client.
		FetchUrl: IsolateURL(host, ns, string(f.Digest)),
	}

	if f.Size != nil {
		a.Size = *f.Size
	}

	switch path.Ext(relPath) {
	case ".txt":
		a.ContentType = "plain/text"
	case ".png":
		a.ContentType = "image/png"
	}

	return a
}

// IsolateURL returns a fetch URL for an isolated object.
func IsolateURL(host, ns, digest string) string {
	return fmt.Sprintf("isolate://%s/%s/%s", host, ns, digest)
}

var isolateURLre = regexp.MustCompile(`^isolate://([^/]+)/([^/]+)/(.+)`)

// ParseIsolateURL parses an isolate URL. It is a reverse of IsolateURL.
func ParseIsolateURL(s string) (host, ns, digest string, err error) {
	m := isolateURLre.FindStringSubmatch(s)
	if m == nil {
		err = errors.Reason("does not match %s", isolateURLre).Err()
		return
	}

	host = m[1]
	ns = m[2]
	digest = m[3]
	return
}

// NormalizeIsolatedPath converts the isolated path to the canonical form.
func NormalizeIsolatedPath(p string) string {
	return path.Clean(strings.ReplaceAll(p, "\\", "/"))
}
