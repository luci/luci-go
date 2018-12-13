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

package cli

import (
	"context"
	"net/http"
	"testing"

	config "go.chromium.org/luci/common/api/luci_config/config/v1"
	"go.chromium.org/luci/gce/appengine/testing/roundtripper"
)

func TestProcessResponses(t *testing.T) {
	var tCases = []struct {
		name                string
		responses           []responseWithPath
		errShouldBeNil      bool
		expectedResultCount int
	}{
		{"no responses", []responseWithPath{}, true, 0},
		// TODO(garymm): Add more test cases before submission.
	}
	for _, tc := range tCases {
		t.Run(tc.name, func(t *testing.T) {
			c := make(chan responseWithPath, len(tc.responses))
			go func() {
				for _, resp := range tc.responses {
					c <- resp
				}
				close(c)
			}()
			res, err := processResponses(c)
			if tc.errShouldBeNil && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if tc.expectedResultCount != len(res.IndividualResults) {
				t.Errorf("Unexpected count of results: %v", res)
			}
		})
	}
}

func TestPushResponses(t *testing.T) {
	// TODO(garymm): Add actual assertions before submission.
	rt := &roundtripper.JSONRoundTripper{}
	svc, err := config.New(&http.Client{Transport: rt})
	if err != nil {
		t.Errorf("Failed to create config service: %v", err)
	}
	vr := &validateRun{}
	vr.init(Parameters{}, true)
	vr.configSet = "projects/foo"
	c := make(chan responseWithPath) // , len(tc.responses)
	var configFiles []*config.LuciConfigValidateConfigRequestMessageFile
	vr.pushResponses(context.Background(), svc, configFiles, c)
}
