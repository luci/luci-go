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

package lucicfg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"go.chromium.org/luci/common/api/luci_config/config/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfigSet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("With temp dir", t, func() {
		tmp := t.TempDir()
		path := func(p ...string) string {
			return filepath.Join(append([]string{tmp}, p...)...)
		}

		So(os.Mkdir(path("subdir"), 0700), ShouldBeNil)
		So(os.WriteFile(path("a.cfg"), []byte("a\n"), 0600), ShouldBeNil)
		So(os.WriteFile(path("subdir", "b.cfg"), []byte("b\n"), 0600), ShouldBeNil)

		Convey("Reading", func() {
			Convey("Success", func() {
				cfg, err := ReadConfigSet(tmp, "set name")
				So(err, ShouldBeNil)
				So(cfg, ShouldResemble, ConfigSet{
					Name: "set name",
					Data: map[string][]byte{
						"a.cfg":        []byte("a\n"),
						"subdir/b.cfg": []byte("b\n"),
					},
				})

				So(cfg.Files(), ShouldResemble, []string{
					"a.cfg",
					"subdir/b.cfg",
				})
			})

			Convey("Missing dir", func() {
				_, err := ReadConfigSet(path("unknown"), "zzz")
				So(err, ShouldNotBeNil)
			})
		})
	})

	Convey("Validation", t, func() {
		const configSetName = "config set name"

		validator := testValidator{
			res: []*ValidationMessage{
				{Severity: "ERROR", Text: "Boo"},
			},
		}

		cfg := ConfigSet{
			Name: configSetName,
			Data: map[string][]byte{
				"a.cfg": []byte("aaa"),
				"b.cfg": {0, 1, 2},
			},
		}

		So(cfg.Validate(ctx, &validator), ShouldResemble, &ValidationResult{
			ConfigSet: configSetName,
			Messages:  validator.res,
		})

		So(validator.req, ShouldResemble, &ValidationRequest{
			ConfigSet: configSetName,
			Files: []*config.LuciConfigValidateConfigRequestMessageFile{
				{Path: "a.cfg", Content: "YWFh"},
				{Path: "b.cfg", Content: "AAEC"},
			},
		})
	})

	Convey("RPC error", t, func() {
		validator := testValidator{
			err: fmt.Errorf("BOOM"),
		}

		cfg := ConfigSet{
			Name: "set",
			Data: map[string][]byte{"a.cfg": []byte("aaa")},
		}

		res := cfg.Validate(ctx, &validator)
		So(res, ShouldResemble, &ValidationResult{
			ConfigSet: "set",
			Failed:    true,
			RPCError:  "BOOM",
		})

		// This is considered overall failure.
		err := res.OverallError(false)
		So(err, ShouldErrLike, "BOOM")
		So(res.Failed, ShouldBeTrue)
	})

	Convey("Overall error check", t, func() {
		result := func(level ...string) *ValidationResult {
			res := &ValidationResult{}
			for _, l := range level {
				res.Messages = append(res.Messages, &ValidationMessage{
					Severity: l,
					Text:     "boo",
				})
			}
			return res
		}

		// Fail on warnings = false.
		So(result().OverallError(false), ShouldBeNil)
		So(result("INFO", "WARNING").OverallError(false), ShouldBeNil)
		So(result("INFO", "ERROR").OverallError(false), ShouldErrLike, "some files were invalid")

		// Fail on warnings = true.
		So(result().OverallError(true), ShouldBeNil)
		So(result("INFO").OverallError(true), ShouldBeNil)
		So(result("INFO", "WARNING", "ERROR").OverallError(true), ShouldErrLike, "some files were invalid")
		So(result("INFO", "WARNING").OverallError(true), ShouldErrLike, "some files had validation warnings")
	})
}

func TestRemoteValidator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	testReq := &ValidationRequest{
		ConfigSet: "config-set",
		Files: []*config.LuciConfigValidateConfigRequestMessageFile{
			{
				Path:    "a",
				Content: "aaa",
			},
			{
				Path:    "b",
				Content: "bb",
			},
			{
				Path:    "c",
				Content: "cccccc",
			},
			{
				Path:    "d",
				Content: "ddddd",
			},
		},
	}

	Convey("Splits requests, collects messages", t, func() {
		var reqs []*ValidationRequest
		var lock sync.Mutex

		val := &remoteValidator{
			requestSizeLimitBytes: 6,
			validateConfig: func(ctx context.Context, req *ValidationRequest) (*config.LuciConfigValidateConfigResponseMessage, error) {
				lock.Lock()
				reqs = append(reqs, req)
				lock.Unlock()

				if req.ConfigSet != "config-set" {
					panic("bad ConfigSet")
				}

				var messages []*ValidationMessage
				for _, f := range req.Files {
					messages = append(messages, &ValidationMessage{
						Path: f.Path,
						Text: fmt.Sprintf("Boom in %s", f.Path),
					})
				}

				return &config.LuciConfigValidateConfigResponseMessage{
					Messages: messages,
				}, nil
			},
		}

		msg, err := val.Validate(ctx, testReq)
		So(err, ShouldBeNil)
		So(msg, ShouldResemble, []*ValidationMessage{
			{Path: "a", Text: "Boom in a"},
			{Path: "b", Text: "Boom in b"},
			{Path: "c", Text: "Boom in c"},
			{Path: "d", Text: "Boom in d"},
		})

		var sets []string
		for _, req := range reqs {
			var reqSize int
			var names []string
			for _, f := range req.Files {
				reqSize += len(f.Content)
				names = append(names, f.Path)
			}
			sets = append(sets, strings.Join(names, "+"))
			So(reqSize, ShouldBeLessThanOrEqualTo, val.requestSizeLimitBytes)
		}
		sort.Strings(sets)
		So(sets, ShouldResemble, []string{"b+a", "c", "d"})
		Convey("Single file too large", func() {
			val.requestSizeLimitBytes = 5
			_, err := val.Validate(ctx, testReq)
			So(err, ShouldErrLike, "the size of file \"c\" is 6")
		})
	})

	Convey("Handles errors", t, func() {
		val := &remoteValidator{
			requestSizeLimitBytes: 6,

			validateConfig: func(ctx context.Context, req *ValidationRequest) (*config.LuciConfigValidateConfigResponseMessage, error) {
				if req.ConfigSet != "config-set" {
					panic("bad ConfigSet")
				}

				var messages []*ValidationMessage
				for _, f := range req.Files {
					if f.Path == "c" || f.Path == "d" {
						return nil, fmt.Errorf("fake error")
					}
					messages = append(messages, &ValidationMessage{
						Path: f.Path,
						Text: fmt.Sprintf("Boom in %s", f.Path),
					})
				}

				return &config.LuciConfigValidateConfigResponseMessage{
					Messages: messages,
				}, nil
			},
		}

		msg, err := val.Validate(ctx, testReq)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "fake error (and 1 other error)")
		So(msg, ShouldResemble, []*ValidationMessage{
			{Path: "a", Text: "Boom in a"},
			{Path: "b", Text: "Boom in b"},
		})
	})
}

type testValidator struct {
	req *ValidationRequest   // captured request
	res []*ValidationMessage // a reply to send
	err error                // an RPC error
}

func (t *testValidator) Validate(ctx context.Context, req *ValidationRequest) ([]*ValidationMessage, error) {
	t.req = req
	return t.res, t.err
}
