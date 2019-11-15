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
	"path/filepath"
	"strings"

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
func IsolatedFileToArtifact(srv, ns, path string, f *isolated.File) *pb.Artifact {
	a := &pb.Artifact{
		Name:     path,
		FetchUrl: fmt.Sprintf("isolate://%s/%s/%s", srv, ns, f.Digest),
	}

	if f.Link != nil {
		return a
	}

	if f.Size != nil {
		a.Size = *f.Size
	}

	switch filepath.Ext(path) {
	case ".txt":
		a.ContentType = "plain/text"
	case ".png":
		a.ContentType = "image/png"
	}

	return a
}
