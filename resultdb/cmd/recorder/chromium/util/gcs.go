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
	"regexp"

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

var gcsPathRe = regexp.MustCompile("^gs://([a-z0-9_.-]+)/(.*)")

// AsGCSOutput returns an artifact corresponding to the path provided if treated as a GCS object.
func AsGCSOutput(name, path string) *pb.Artifact {
	if m := gcsPathRe.FindStringSubmatch(path); m != nil {
		return &pb.Artifact{
			Name:     name,
			FetchUrl: path,
			ViewUrl:  fmt.Sprintf(
				"https://console.developers.google.com/m/cloudstorage/b/%s/o/%s", m[1], m[2]),
		}
	}
	return nil
}
