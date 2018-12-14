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

package buildbucket

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/protobuf/jsonpb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

// We put this here because _test.go files are sometimes not built.
var TestCases = []string{"linux-rel"}

// GetTestBuild returns a debug build from testdata.
func GetTestBuild(c context.Context, relDir, name string) (*buildbucketpb.Build, error) {
	fname := fmt.Sprintf("%s.build.jsonpb", name)
	path := filepath.Join(relDir, "testdata", fname)
	f, err := os.Open(path)
	f.Seek(4, 0) // Remove the jsonp header.
	if err != nil {
		return nil, err
	}
	result := &buildbucketpb.Build{}
	return result, jsonpb.Unmarshal(f, result)
}
