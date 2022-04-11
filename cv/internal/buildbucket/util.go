// Copyright 2022 The LUCI Authors.
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

package buildbucket

import (
	"fmt"
	"strings"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

// ParseBuilderID converts a builderID string like "project/bucket/builder" to
// a protobuf BuilderID.
func ParseBuilderID(s string) (*buildbucketpb.BuilderID, error) {
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return nil, fmt.Errorf("builder id in unexpected format: %q, expected <project>/<bucket>/<builder>", s)
	}
	return &buildbucketpb.BuilderID{
		Project: parts[0],
		Bucket:  parts[1],
		Builder: parts[2],
	}, nil
}
