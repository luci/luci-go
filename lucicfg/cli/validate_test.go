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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	config "go.chromium.org/luci/common/api/luci_config/config/v1"

	. "github.com/smartystreets/goconvey/convey"
)

type ValidationMsg = config.ComponentsConfigEndpointValidationMessage

func TestProcessResponse(t *testing.T) {
	var tCases = []struct {
		name                 string
		inputMessages        []*ValidationMsg
		inputErr             error
		errShouldBeNil       bool
		expectedMessageCount int
	}{
		{"responses", []*ValidationMsg{
			{
				Path:     "foo.cfg",
				Severity: "ERROR",
				Text:     "I'm afraid I can't do that Dave",
			},
			{
				Path:     "bar.cfg",
				Severity: "WARN",
				Text:     "Uh oh",
			},
		}, nil, false, 2},
		{"no responses", []*ValidationMsg{}, nil, true, 0},
		{"no responses with input error", []*ValidationMsg{}, fmt.Errorf("!"), false, 0},
		{"responses with input error", []*ValidationMsg{{
			Path:     "foo.cfg",
			Severity: "ERROR",
			Text:     "I'm afraid I can't do that Dave",
		}}, fmt.Errorf("!"), false, 0},
	}
	ctx := context.Background()
	for _, tc := range tCases {
		Convey(tc.name, t, func() {
			resp := config.LuciConfigValidateConfigResponseMessage{
				Messages: tc.inputMessages,
			}
			res, err := processResponse(ctx, &resp, tc.inputErr)
			if tc.errShouldBeNil {
				So(err, ShouldBeNil)
			} else {
				So(err, ShouldNotBeNil)
			}
			So(tc.expectedMessageCount, ShouldEqual, len(res.ErrorMessages))
		})
	}
}

func TestConstructRequest(t *testing.T) {
	configDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Errorf("Failed to create temp dir: %v", err)
		return
	}
	defer os.RemoveAll(configDir)
	if err = ioutil.WriteFile(filepath.Join(configDir, "a.cfg"), []byte("a\n"), 0600); err != nil {
		t.Errorf("Failed to write a.cfg: %v", err)
		return
	}
	subdir := filepath.Join(configDir, "subdir")
	if err = os.Mkdir(subdir, 0700); err != nil {
		t.Errorf("Failed to MkDir %s: %v", subdir, err)
		return
	}
	if err = ioutil.WriteFile(filepath.Join(subdir, "b.cfg"), []byte("b\n"), 0600); err != nil {
		t.Errorf("Failed to write b.cfg: %v", err)
		return
	}
	expectedPaths := []string{"a.cfg", "subdir/b.cfg"}

	vr := &validateRun{
		configSet: "arbitrary",
		configDir: configDir,
	}
	Convey("constructRequest succeeds", t, func() {
		req, err := vr.constructRequest()
		So(err, ShouldBeNil)
		Convey("config set should match expected", func() {
			So(vr.configSet, ShouldEqual, req.ConfigSet)
		})
		Convey("request files should be the same length as expectedPaths", func() {
			So(len(req.Files), ShouldEqual, len(expectedPaths))
		})
		// The order of req.Files doesn't really matter, but making this test smart enough
		// to ignore order is probably not worth the effort.
		Convey("request files should match expectedPaths", func() {
			for i, expectedPath := range expectedPaths {
				So(req.Files[i].Path, ShouldEqual, expectedPath)
				So(req.Files[i].Content, ShouldNotBeBlank)
			}
		})
	})
}
