// Copyright 2024 The LUCI Authors.
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

package should

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/buildbucket/proto"
)

func TestResemble(t *testing.T) {
	t.Parallel()

	t.Run("simple", shouldPass(Resemble(100)(100)))
	t.Run("simple fail", shouldFail(Resemble(100)(101), "Diff"))

	t.Run("simple proto", shouldPass(
		Resemble(&buildbucketpb.Build{Id: 12345})(&buildbucketpb.Build{Id: 12345})))

	props, err := structpb.NewStruct(map[string]interface{}{
		"heyo": 100,
	})
	if err != nil {
		t.Fatal("could not make struct", err)
	}
	t.Run("struct proto fail", shouldFail(
		Resemble(&buildbucketpb.Build{Id: 12345, Input: &buildbucketpb.Build_Input{
			Properties: props,
		}})(&buildbucketpb.Build{Id: 12345}), "Diff"))
}
