// Copyright 2017 The LUCI Authors.
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

package testutil

import (
	bbapi "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"

	"go.chromium.org/luci/luci_notify/buildbucket"
)

// CreateTestBuild creates a minimal dummy buildbucket.BuildInfo struct for
// use in testing.
func CreateTestBuild(createTime int64, bucket, name, result string) *buildbucket.BuildInfo {
	return &buildbucket.BuildInfo{
		Build: bbapi.ApiCommonBuildMessage{
			Bucket:    bucket,
			CreatedTs: createTime,
			Result:    result,
		},
		Hostname: "bb.example.com",
		Parameters: buildbucket.BuildParameters{
			BuilderName: name,
		},
	}
}
