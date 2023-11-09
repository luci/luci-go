// Copyright 2023 The LUCI Authors.
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
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
)

func GetCompileRerunBuilder(project string) (*bbpb.BuilderID, error) {
	if project != "chromium" {
		return nil, errors.Reason("unsupported project: %q", project).Err()
	}
	return &bbpb.BuilderID{
		Project: "chromium",
		Bucket:  "findit",
		Builder: "gofindit-culprit-verification",
	}, nil
}

func GetTestRerunBuilder(project string) (*bbpb.BuilderID, error) {
	if project != "chromium" {
		return nil, errors.Reason("unsupported project: %q", project).Err()
	}
	return &bbpb.BuilderID{
		Project: "chromium",
		Bucket:  "findit",
		Builder: "test-single-revision",
	}, nil
}
