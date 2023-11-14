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
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/klauspost/compress/gzip"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	legacy_config "go.chromium.org/luci/common/api/luci_config/config/v1"
	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/proto/config"
	configpb "go.chromium.org/luci/config_service/proto"

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

func TestRemoteValidator(t *testing.T) {
	t.Parallel()

	Convey("Remote Validator", t, func(c C) {
		ctx := context.Background()
		ctrl := gomock.NewController(t)
		mockClient := configpb.NewMockConfigsClient(ctrl)

		validator := &remoteValidator{
			cfgClient: mockClient,
		}

		cs := ConfigSet{
			Name: "example-proj",
			Data: map[string][]byte{
				"foo.cfg": []byte("This is the config content"),
			},
		}
		validationMsgs := []*config.ValidationResult_Message{
			{
				Path:     "foo.cfg",
				Severity: config.ValidationResult_ERROR,
				Text:     "bad config syntax",
			},
		}

		Convey("empty config set", func() {
			cs.Data = nil
			res, err := validator.Validate(ctx, cs)
			So(err, ShouldBeNil)
			So(res, ShouldBeEmpty)
		})
		Convey("successfully validated", func() {
			mockClient.EXPECT().ValidateConfigs(gomock.Any(), proto.MatcherEqual(&configpb.ValidateConfigsRequest{
				ConfigSet: cs.Name,
				FileHashes: []*configpb.ValidateConfigsRequest_FileHash{
					{
						Path:   "foo.cfg",
						Sha256: "fd243f0466e35bcc9146b012415e11c88627a2a7c31a7fb121c5a8e99401e417",
					},
				},
			})).Return(&config.ValidationResult{
				Messages: validationMsgs,
			}, nil)
			res, err := validator.Validate(ctx, cs)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, validationMsgs)
		})
		Convey("unvalidatable files", func() {
			cs.Data["unvalidatable.cfg"] = []byte("some content")
			st, err := status.New(codes.InvalidArgument, "invalid validate config request").WithDetails(&configpb.BadValidationRequestFixInfo{
				UnvalidatableFiles: []string{"unvalidatable.cfg"},
			})
			So(err, ShouldBeNil)
			mockClient.EXPECT().ValidateConfigs(gomock.Any(), proto.MatcherEqual(&configpb.ValidateConfigsRequest{
				ConfigSet: cs.Name,
				FileHashes: []*configpb.ValidateConfigsRequest_FileHash{
					{
						Path:   "foo.cfg",
						Sha256: "fd243f0466e35bcc9146b012415e11c88627a2a7c31a7fb121c5a8e99401e417",
					},
					{
						Path:   "unvalidatable.cfg",
						Sha256: "290f493c44f5d63d06b374d0a5abd292fae38b92cab2fae5efefe1b0e9347f56",
					},
				},
			})).Return(nil, st.Err())
			// Retry after stripping out `unvalidatable.cfg`
			mockClient.EXPECT().ValidateConfigs(gomock.Any(), proto.MatcherEqual(&configpb.ValidateConfigsRequest{
				ConfigSet: cs.Name,
				FileHashes: []*configpb.ValidateConfigsRequest_FileHash{
					{
						Path:   "foo.cfg",
						Sha256: "fd243f0466e35bcc9146b012415e11c88627a2a7c31a7fb121c5a8e99401e417",
					},
				},
			})).Return(&config.ValidationResult{
				Messages: validationMsgs,
			}, nil)
			res, err := validator.Validate(ctx, cs)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, validationMsgs)
		})
		Convey("upload files", func(c C) {
			Convey("succeed", func() {
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					c.So(r.Method, ShouldEqual, http.MethodPut)
					c.So(r.Header.Get("Content-Encoding"), ShouldEqual, "gzip")
					c.So(r.Header.Get("x-goog-content-length-range"), ShouldEqual, "0,10240")
					compressed, err := io.ReadAll(r.Body)
					c.So(err, ShouldBeNil)
					reader, err := gzip.NewReader(bytes.NewBuffer(compressed))
					c.So(err, ShouldBeNil)
					config, err := io.ReadAll(reader)
					c.So(err, ShouldBeNil)
					c.So(string(config), ShouldEqual, "This is the config content")
					w.WriteHeader(http.StatusOK)
				}))
				defer ts.Close()
				st, err := status.New(codes.InvalidArgument, "invalid validate config request").WithDetails(&configpb.BadValidationRequestFixInfo{
					UploadFiles: []*configpb.BadValidationRequestFixInfo_UploadFile{
						{
							Path:          "foo.cfg",
							SignedUrl:     ts.URL,
							MaxConfigSize: 10240,
						},
					},
				})
				So(err, ShouldBeNil)
				mockClient.EXPECT().ValidateConfigs(gomock.Any(), proto.MatcherEqual(&configpb.ValidateConfigsRequest{
					ConfigSet: cs.Name,
					FileHashes: []*configpb.ValidateConfigsRequest_FileHash{
						{
							Path:   "foo.cfg",
							Sha256: "fd243f0466e35bcc9146b012415e11c88627a2a7c31a7fb121c5a8e99401e417",
						},
					},
				})).Return(nil, st.Err()).
					Return(&config.ValidationResult{
						Messages: validationMsgs,
					}, nil)
				res, err := validator.Validate(ctx, cs)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, validationMsgs)
			})

			Convey("no file unvalidatable", func() {
				cs.Data = map[string][]byte{
					"unvalidatable.cfg": []byte("some content"),
				}
				st, err := status.New(codes.InvalidArgument, "invalid validate config request").WithDetails(&configpb.BadValidationRequestFixInfo{
					UnvalidatableFiles: []string{"unvalidatable.cfg"},
				})
				So(err, ShouldBeNil)
				mockClient.EXPECT().ValidateConfigs(gomock.Any(), proto.MatcherEqual(&configpb.ValidateConfigsRequest{
					ConfigSet: cs.Name,
					FileHashes: []*configpb.ValidateConfigsRequest_FileHash{
						{
							Path:   "unvalidatable.cfg",
							Sha256: "290f493c44f5d63d06b374d0a5abd292fae38b92cab2fae5efefe1b0e9347f56",
						},
					},
				})).Return(nil, st.Err())
				res, err := validator.Validate(ctx, cs)
				So(err, ShouldBeNil)
				So(res, ShouldBeEmpty)
			})
			Convey("failed", func() {
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
					fmt.Fprintf(w, "config is too large")
				}))
				defer ts.Close()
				st, err := status.New(codes.InvalidArgument, "invalid validate config request").WithDetails(&configpb.BadValidationRequestFixInfo{
					UploadFiles: []*configpb.BadValidationRequestFixInfo_UploadFile{
						{
							Path:          "foo.cfg",
							SignedUrl:     ts.URL,
							MaxConfigSize: 10240,
						},
					},
				})
				So(err, ShouldBeNil)
				mockClient.EXPECT().ValidateConfigs(gomock.Any(), proto.MatcherEqual(&configpb.ValidateConfigsRequest{
					ConfigSet: cs.Name,
					FileHashes: []*configpb.ValidateConfigsRequest_FileHash{
						{
							Path:   "foo.cfg",
							Sha256: "fd243f0466e35bcc9146b012415e11c88627a2a7c31a7fb121c5a8e99401e417",
						},
					},
				})).Return(nil, st.Err())
				res, err := validator.Validate(ctx, cs)
				So(err, ShouldErrLike, "failed to upload file")
				So(res, ShouldBeEmpty)
			})
		})
		Convey("failed to call LUCI Config", func() {
			mockClient.EXPECT().ValidateConfigs(gomock.Any(), proto.MatcherEqual(&configpb.ValidateConfigsRequest{
				ConfigSet: cs.Name,
				FileHashes: []*configpb.ValidateConfigsRequest_FileHash{
					{
						Path:   "foo.cfg",
						Sha256: "fd243f0466e35bcc9146b012415e11c88627a2a7c31a7fb121c5a8e99401e417",
					},
				},
			})).Return(nil, status.Error(codes.InvalidArgument, "invalid validate config request"))
			res, err := validator.Validate(ctx, cs)
			So(err, ShouldErrLike, "failed to call LUCI Config")
			So(res, ShouldBeEmpty)
		})
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
