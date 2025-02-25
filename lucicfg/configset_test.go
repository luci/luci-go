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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/klauspost/compress/gzip"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	configpb "go.chromium.org/luci/config_service/proto"
)

func TestConfigSet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("With temp dir", t, func(t *ftt.Test) {
		tmp := t.TempDir()
		path := func(p ...string) string {
			return filepath.Join(append([]string{tmp}, p...)...)
		}

		assert.Loosely(t, os.Mkdir(path("subdir"), 0700), should.BeNil)
		assert.Loosely(t, os.WriteFile(path("a.cfg"), []byte("a\n"), 0600), should.BeNil)
		assert.Loosely(t, os.WriteFile(path("subdir", "b.cfg"), []byte("b\n"), 0600), should.BeNil)

		t.Run("Reading", func(t *ftt.Test) {
			t.Run("Success", func(t *ftt.Test) {
				cfg, err := ReadConfigSet(tmp, "set name")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, cfg, should.Match(ConfigSet{
					Name: "set name",
					Data: map[string][]byte{
						"a.cfg":        []byte("a\n"),
						"subdir/b.cfg": []byte("b\n"),
					},
				}))

				assert.Loosely(t, cfg.Files(), should.Match([]string{
					"a.cfg",
					"subdir/b.cfg",
				}))
			})

			t.Run("Missing dir", func(t *ftt.Test) {
				_, err := ReadConfigSet(path("unknown"), "zzz")
				assert.Loosely(t, err, should.NotBeNil)
			})
		})
	})

	ftt.Run("Validation", t, func(t *ftt.Test) {
		const configSetName = "config set name"

		errorMsg := &config.ValidationResult_Message{
			Severity: config.ValidationResult_ERROR,
			Text:     "Boo",
		}

		validator := testValidator{
			res: []*config.ValidationResult_Message{errorMsg},
		}

		cfgSet := ConfigSet{
			Name: configSetName,
			Data: map[string][]byte{
				"a.cfg": []byte("aaa"),
				"b.cfg": {0, 1, 2},
			},
		}

		assert.Loosely(t, cfgSet.Validate(ctx, &validator), should.Match(&ValidationResult{
			ConfigSet: configSetName,
			Messages:  []ValidationMessage{{errorMsg}},
		}))

		assert.Loosely(t, validator.cs, should.Match(cfgSet))
	})

	ftt.Run("RPC error", t, func(t *ftt.Test) {
		validator := testValidator{
			err: fmt.Errorf("BOOM"),
		}

		cfg := ConfigSet{
			Name: "set",
			Data: map[string][]byte{"a.cfg": []byte("aaa")},
		}

		res := cfg.Validate(ctx, &validator)
		assert.Loosely(t, res, should.Match(&ValidationResult{
			ConfigSet: "set",
			Failed:    true,
			RPCError:  "BOOM",
		}))

		// This is considered overall failure.
		err := res.OverallError(false)
		assert.Loosely(t, err, should.ErrLike("BOOM"))
		assert.Loosely(t, res.Failed, should.BeTrue)
	})

	ftt.Run("Overall error check", t, func(t *ftt.Test) {
		result := func(level ...config.ValidationResult_Severity) *ValidationResult {
			res := &ValidationResult{}
			for _, l := range level {
				res.Messages = append(res.Messages, ValidationMessage{&config.ValidationResult_Message{
					Severity: l,
					Text:     "boo",
				}})
			}
			return res
		}

		// Fail on warnings = false.
		assert.Loosely(t, result().OverallError(false), should.BeNil)
		assert.Loosely(t, result(config.ValidationResult_INFO, config.ValidationResult_WARNING).OverallError(false), should.BeNil)
		assert.Loosely(t, result(config.ValidationResult_INFO, config.ValidationResult_ERROR).OverallError(false), should.ErrLike("some files were invalid"))

		// Fail on warnings = true.
		assert.Loosely(t, result().OverallError(true), should.BeNil)
		assert.Loosely(t, result(config.ValidationResult_INFO).OverallError(true), should.BeNil)
		assert.Loosely(t, result(config.ValidationResult_INFO, config.ValidationResult_WARNING, config.ValidationResult_ERROR).OverallError(true), should.ErrLike("some files were invalid"))
		assert.Loosely(t, result(config.ValidationResult_INFO, config.ValidationResult_WARNING).OverallError(true), should.ErrLike("some files had validation warnings"))
	})
}

func TestRemoteValidator(t *testing.T) {
	t.Parallel()

	ftt.Run("Remote Validator", t, func(c *ftt.Test) {
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

		c.Run("empty config set", func(c *ftt.Test) {
			cs.Data = nil
			res, err := validator.Validate(ctx, cs)
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, res, should.BeEmpty)
		})
		c.Run("successfully validated", func(c *ftt.Test) {
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
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, res, should.Match(validationMsgs))
		})
		c.Run("unvalidatable files", func(c *ftt.Test) {
			cs.Data["unvalidatable.cfg"] = []byte("some content")
			st, err := status.New(codes.InvalidArgument, "invalid validate config request").WithDetails(&configpb.BadValidationRequestFixInfo{
				UnvalidatableFiles: []string{"unvalidatable.cfg"},
			})
			assert.Loosely(c, err, should.BeNil)
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
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, res, should.Match(validationMsgs))
		})
		c.Run("upload files", func(c *ftt.Test) {
			c.Run("succeed", func(c *ftt.Test) {
				ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Loosely(c, r.Method, should.Equal(http.MethodPut))
					assert.Loosely(c, r.Header.Get("Content-Encoding"), should.Equal("gzip"))
					assert.Loosely(c, r.Header.Get("x-goog-content-length-range"), should.Equal("0,10240"))
					compressed, err := io.ReadAll(r.Body)
					assert.Loosely(c, err, should.BeNil)
					reader, err := gzip.NewReader(bytes.NewBuffer(compressed))
					assert.Loosely(c, err, should.BeNil)
					config, err := io.ReadAll(reader)
					assert.Loosely(c, err, should.BeNil)
					assert.Loosely(c, string(config), should.Equal("This is the config content"))
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
				assert.Loosely(c, err, should.BeNil)
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
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, res, should.Match(validationMsgs))
			})

			c.Run("no file unvalidatable", func(c *ftt.Test) {
				cs.Data = map[string][]byte{
					"unvalidatable.cfg": []byte("some content"),
				}
				st, err := status.New(codes.InvalidArgument, "invalid validate config request").WithDetails(&configpb.BadValidationRequestFixInfo{
					UnvalidatableFiles: []string{"unvalidatable.cfg"},
				})
				assert.Loosely(c, err, should.BeNil)
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
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, res, should.BeEmpty)
			})
			c.Run("failed", func(c *ftt.Test) {
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
				assert.Loosely(c, err, should.BeNil)
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
				assert.Loosely(c, err, should.ErrLike("failed to upload file"))
				assert.Loosely(c, res, should.BeEmpty)
			})
		})
		c.Run("failed to call LUCI Config", func(c *ftt.Test) {
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
			assert.Loosely(c, err, should.ErrLike("failed to call LUCI Config"))
			assert.Loosely(c, res, should.BeEmpty)
		})
	})
}

func TestValidationResultJSON(t *testing.T) {
	t.Parallel()

	res := ValidationResult{
		ConfigSet: "config_set",
		Failed:    true,
		Messages: []ValidationMessage{
			{&config.ValidationResult_Message{
				Path:     "path1",
				Severity: config.ValidationResult_ERROR,
				Text:     "error",
			}},
			{&config.ValidationResult_Message{
				Path:     "path2",
				Severity: config.ValidationResult_WARNING,
				Text:     "warn",
			}},
		},
		RPCError: "rcp error",
	}

	blob, err := json.MarshalIndent(&res, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	expected := `{
  "config_set": "config_set",
  "failed": true,
  "messages": [
    {
      "path": "path1",
      "severity": "ERROR",
      "text": "error"
    },
    {
      "path": "path2",
      "severity": "WARNING",
      "text": "warn"
    }
  ],
  "rpc_error": "rcp error"
}`

	if string(blob) != expected {
		t.Fatalf("Got:\n%s", string(blob))
	}
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
