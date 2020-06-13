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

package util

import (
	"fmt"
	"path"
	"strings"

	pb "go.chromium.org/luci/resultdb/proto/v1"
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

// NormalizeIsolatedPath converts the isolated path to the canonical form.
func NormalizeIsolatedPath(p string) string {
	return path.Clean(strings.ReplaceAll(p, "\\", "/"))
}

// IsolateServerToHost converts an Isolate URL to a hostname.
func IsolateServerToHost(server string) string {
	host := server
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimRight(host, "/")
	return host
}
