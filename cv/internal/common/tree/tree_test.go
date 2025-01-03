// Copyright 2021 The LUCI Authors.
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

package tree

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
)

func TestTreeName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		config         *cfgpb.Verifiers_TreeStatus
		expectedResult string
	}{
		{
			name: "tree name specified",
			config: &cfgpb.Verifiers_TreeStatus{
				TreeName: "my-tree",
			},
			expectedResult: "my-tree",
		},
		{
			name: "url specified",
			config: &cfgpb.Verifiers_TreeStatus{
				Url: "https://my-tree-status.appspot.com",
			},
			expectedResult: "my-tree",
		},
		{
			name: "both specified",
			config: &cfgpb.Verifiers_TreeStatus{
				TreeName: "my-tree",
				Url:      "https://my-other-tree-status.appspot.com",
			},
			expectedResult: "my-tree",
		},
		{
			name:           "not specified",
			config:         &cfgpb.Verifiers_TreeStatus{},
			expectedResult: "",
		},
	}
	for _, testcase := range tests {
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()
			assert.That(t, TreeName(testcase.config), should.Equal(testcase.expectedResult))
		})
	}
}
