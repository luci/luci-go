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
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	legacy_config "go.chromium.org/luci/common/api/luci_config/config/v1"
	"go.chromium.org/luci/common/proto/config"

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
			res: []*config.ValidationResult_Message{
				{Severity: config.ValidationResult_ERROR, Text: "Boo"},
			},
		}

		cfgSet := ConfigSet{
			Name: configSetName,
			Data: map[string][]byte{
				"a.cfg": []byte("aaa"),
				"b.cfg": {0, 1, 2},
			},
		}

		So(cfgSet.Validate(ctx, &validator), ShouldResemble, &ValidationResult{
			ConfigSet: configSetName,
			Messages:  validator.res,
		})

		So(validator.cs, ShouldResemble, cfgSet)
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
		result := func(level ...config.ValidationResult_Severity) *ValidationResult {
			res := &ValidationResult{}
			for _, l := range level {
				res.Messages = append(res.Messages, &config.ValidationResult_Message{
					Severity: l,
					Text:     "boo",
				})
			}
			return res
		}

		// Fail on warnings = false.
		So(result().OverallError(false), ShouldBeNil)
		So(result(config.ValidationResult_INFO, config.ValidationResult_WARNING).OverallError(false), ShouldBeNil)
		So(result(config.ValidationResult_INFO, config.ValidationResult_ERROR).OverallError(false), ShouldErrLike, "some files were invalid")

		// Fail on warnings = true.
		So(result().OverallError(true), ShouldBeNil)
		So(result(config.ValidationResult_INFO).OverallError(true), ShouldBeNil)
		So(result(config.ValidationResult_INFO, config.ValidationResult_WARNING, config.ValidationResult_ERROR).OverallError(true), ShouldErrLike, "some files were invalid")
		So(result(config.ValidationResult_INFO, config.ValidationResult_WARNING).OverallError(true), ShouldErrLike, "some files had validation warnings")
	})
}

func TestLegacyRemoteValidator(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	mustBase64Decode := func(s string) []byte {
		ret, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			panic(err)
		}
		return ret
	}

	cfgSet := ConfigSet{
		Name: "config-set",
		Data: map[string][]byte{
			// "aaaaaaaa", "bbbb", "cccccccccccc", "dddddddd" will be the actual
			// content send to LUCI Config
			"a.cfg": mustBase64Decode("aaaaaaaa"),
			"b.cfg": mustBase64Decode("bbbb"),
			"c.cfg": mustBase64Decode("cccccccccccc"),
			"d.cfg": mustBase64Decode("dddddddd"),
		},
	}

	Convey("Splits requests, collects messages", t, func() {
		var reqs []*legacy_config.LuciConfigValidateConfigRequestMessage
		var lock sync.Mutex

		val := &legacyRemoteValidator{
			requestSizeLimitBytes: 12,
			validateConfig: func(ctx context.Context, req *legacy_config.LuciConfigValidateConfigRequestMessage) (*legacy_config.LuciConfigValidateConfigResponseMessage, error) {
				lock.Lock()
				reqs = append(reqs, req)
				lock.Unlock()

				if req.ConfigSet != "config-set" {
					panic("bad ConfigSet")
				}

				var messages []*legacy_config.ComponentsConfigEndpointValidationMessage
				for _, f := range req.Files {
					messages = append(messages, &legacy_config.ComponentsConfigEndpointValidationMessage{
						Path:     f.Path,
						Severity: "ERROR",
						Text:     fmt.Sprintf("Boom in %s", f.Path),
					})
				}

				return &legacy_config.LuciConfigValidateConfigResponseMessage{
					Messages: messages,
				}, nil
			},
		}

		msg, err := val.Validate(ctx, cfgSet)
		So(err, ShouldBeNil)
		So(msg, ShouldResembleProto, []*config.ValidationResult_Message{
			{Path: "a.cfg", Severity: config.ValidationResult_ERROR, Text: "Boom in a.cfg"},
			{Path: "b.cfg", Severity: config.ValidationResult_ERROR, Text: "Boom in b.cfg"},
			{Path: "c.cfg", Severity: config.ValidationResult_ERROR, Text: "Boom in c.cfg"},
			{Path: "d.cfg", Severity: config.ValidationResult_ERROR, Text: "Boom in d.cfg"},
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
		So(sets, ShouldResemble, []string{"b.cfg+a.cfg", "c.cfg", "d.cfg"})
		Convey("Single file too large", func() {
			val.requestSizeLimitBytes = 10
			_, err := val.Validate(ctx, cfgSet)
			So(err, ShouldErrLike, "the size of file \"c.cfg\" is 12")
		})
	})

	Convey("Handles errors", t, func() {
		val := &legacyRemoteValidator{
			requestSizeLimitBytes: 12,
			validateConfig: func(ctx context.Context, req *legacy_config.LuciConfigValidateConfigRequestMessage) (*legacy_config.LuciConfigValidateConfigResponseMessage, error) {
				if req.ConfigSet != "config-set" {
					panic("bad ConfigSet")
				}

				var messages []*legacy_config.ComponentsConfigEndpointValidationMessage
				for _, f := range req.Files {
					if f.Path == "c.cfg" || f.Path == "d.cfg" {
						return nil, fmt.Errorf("fake error")
					}
					messages = append(messages, &legacy_config.ComponentsConfigEndpointValidationMessage{
						Path:     f.Path,
						Severity: "ERROR",
						Text:     fmt.Sprintf("Boom in %s", f.Path),
					})
				}

				return &legacy_config.LuciConfigValidateConfigResponseMessage{
					Messages: messages,
				}, nil
			},
		}

		msg, err := val.Validate(ctx, cfgSet)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "fake error (and 1 other error)")
		So(msg, ShouldResembleProto, []*config.ValidationResult_Message{
			{Path: "a.cfg", Severity: config.ValidationResult_ERROR, Text: "Boom in a.cfg"},
			{Path: "b.cfg", Severity: config.ValidationResult_ERROR, Text: "Boom in b.cfg"},
		})
	})
}

type testValidator struct {
	cs  ConfigSet                          // captured config set
	res []*config.ValidationResult_Message // a reply to send
	err error                              // an RPC error
}

func (t *testValidator) Validate(ctx context.Context, cs ConfigSet) ([]*config.ValidationResult_Message, error) {
	t.cs = cs
	return t.res, t.err
}
