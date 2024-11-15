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

package rpcs

import (
	"context"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

func simpliestValidRequest() *apipb.NewTaskRequest {
	return &apipb.NewTaskRequest{
		Name:     "new",
		Priority: 40,
		TaskSlices: []*apipb.TaskSlice{
			{
				Properties: &apipb.TaskProperties{
					GracePeriodSecs:      60,
					ExecutionTimeoutSecs: 300,
					IoTimeoutSecs:        300,
					Command:              []string{"command", "arg"},
					Dimensions: []*apipb.StringPair{
						{
							Key:   "pool",
							Value: "pool",
						},
					},
				},
				ExpirationSecs: 300,
			},
		},
	}
}

func TestValidateNewTask(t *testing.T) {
	t.Parallel()

	ftt.Run("validateNewTask", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		t.Run("empty", func(t *ftt.Test) {
			req := &apipb.NewTaskRequest{}
			err := validateNewTask(ctx, req)
			assert.Loosely(t, err, should.ErrLike("name is required"))
		})

		t.Run("with_properties", func(t *ftt.Test) {
			req := &apipb.NewTaskRequest{
				Name:       "new",
				Properties: &apipb.TaskProperties{},
			}
			err := validateNewTask(ctx, req)
			assert.Loosely(t, err, should.ErrLike("properties is deprecated"))
		})

		t.Run("with_expiration_secs", func(t *ftt.Test) {
			req := &apipb.NewTaskRequest{
				Name:           "new",
				ExpirationSecs: 300,
			}
			err := validateNewTask(ctx, req)
			assert.Loosely(t, err, should.ErrLike("expiration_secs is deprecated"))
		})

		t.Run("parent_task_id", func(t *ftt.Test) {
			req := simpliestValidRequest()
			req.ParentTaskId = "invalid"
			err := validateNewTask(ctx, req)
			assert.Loosely(t, err, should.ErrLike("bad task ID"))
		})

		t.Run("priority", func(t *ftt.Test) {
			t.Run("too_small", func(t *ftt.Test) {
				req := simpliestValidRequest()
				req.Priority = -1
				err := validateNewTask(ctx, req)
				assert.Loosely(t, err, should.ErrLike("must be between 0 and 255"))
			})
			t.Run("too_big", func(t *ftt.Test) {
				req := simpliestValidRequest()
				req.Priority = 500
				err := validateNewTask(ctx, req)
				assert.Loosely(t, err, should.ErrLike("must be between 0 and 255"))
			})
		})

		t.Run("service_account", func(t *ftt.Test) {
			cases := []struct {
				sa  string
				err any
			}{
				{"", nil},
				{"none", nil},
				{"bot", nil},
				{"sa@service-accounts.com", nil},
				{"invalid", "invalid"},
			}
			for _, cs := range cases {
				t.Run(cs.sa, func(t *ftt.Test) {
					req := simpliestValidRequest()
					req.ServiceAccount = cs.sa
					assert.That(t, validateNewTask(ctx, req), should.ErrLike(cs.err))
				})
			}
		})

		t.Run("pubsub_topic", func(t *ftt.Test) {
			cases := []struct {
				name  string
				topic string
				err   any
			}{
				{"empty", "", nil},
				{"valid", "projects/project/topics/topic", nil},
				{"too_long", strings.Repeat("l", maxPubsubLength+1), "too long"},
				{"invalid", "invalid", "must be a well formatted pubsub topic"},
			}
			for _, cs := range cases {
				t.Run(cs.name, func(t *ftt.Test) {
					req := simpliestValidRequest()
					req.PubsubTopic = cs.topic
					assert.That(t, validateNewTask(ctx, req), should.ErrLike(cs.err))
				})
			}
		})

		t.Run("pubsub_userdata", func(t *ftt.Test) {
			req := simpliestValidRequest()
			req.PubsubUserdata = strings.Repeat("l", maxPubsubLength+1)
			err := validateNewTask(ctx, req)
			assert.Loosely(t, err, should.ErrLike("too long"))
		})

		t.Run("bot_ping_tolerance", func(t *ftt.Test) {
			t.Run("too_small", func(t *ftt.Test) {
				req := simpliestValidRequest()
				req.BotPingToleranceSecs = 20
				err := validateNewTask(ctx, req)
				assert.Loosely(t, err, should.ErrLike("must be between"))
			})
			t.Run("too_big", func(t *ftt.Test) {
				req := simpliestValidRequest()
				req.BotPingToleranceSecs = 5000
				err := validateNewTask(ctx, req)
				assert.Loosely(t, err, should.ErrLike("must be between"))
			})
		})

		t.Run("realm", func(t *ftt.Test) {
			req := simpliestValidRequest()
			req.Realm = "invalid"
			err := validateNewTask(ctx, req)
			assert.Loosely(t, err, should.ErrLike("invalid"))
		})

		t.Run("tags", func(t *ftt.Test) {
			req := simpliestValidRequest()
			req.Tags = []string{"invalid"}
			err := validateNewTask(ctx, req)
			assert.Loosely(t, err, should.ErrLike("invalid"))
		})

		t.Run("task_slices", func(t *ftt.Test) {
			t.Run("no_task_slices", func(t *ftt.Test) {
				req := &apipb.NewTaskRequest{
					Name: "new",
				}
				err := validateNewTask(ctx, req)
				assert.Loosely(t, err, should.ErrLike("task_slices is required"))
			})

			t.Run("secret_bytes", func(t *ftt.Test) {
				t.Run("too_long", func(t *ftt.Test) {
					req := &apipb.NewTaskRequest{
						Name: "new",
						TaskSlices: []*apipb.TaskSlice{
							{
								Properties: &apipb.TaskProperties{
									SecretBytes: []byte(strings.Repeat("a", maxSecretBytesLength+1)),
								},
							},
						},
					}
					err := validateNewTask(ctx, req)
					assert.Loosely(t, err, should.ErrLike("exceeding limit"))
				})

				t.Run("unmatch", func(t *ftt.Test) {
					req := &apipb.NewTaskRequest{
						Name:     "new",
						Priority: 40,
						TaskSlices: []*apipb.TaskSlice{
							{
								Properties: &apipb.TaskProperties{
									GracePeriodSecs:      60,
									ExecutionTimeoutSecs: 300,
									IoTimeoutSecs:        300,
									Command:              []string{"command", "arg"},
									Dimensions: []*apipb.StringPair{
										{
											Key:   "pool",
											Value: "pool",
										},
									},
									SecretBytes: []byte("this is a secret"),
								},
								ExpirationSecs: 300,
							},
							{
								Properties: &apipb.TaskProperties{
									GracePeriodSecs:      60,
									ExecutionTimeoutSecs: 300,
									IoTimeoutSecs:        300,
									Command:              []string{"command", "arg"},
									Dimensions: []*apipb.StringPair{
										{
											Key:   "pool",
											Value: "pool",
										},
									},
									SecretBytes: []byte("this is another secret"),
								},
								ExpirationSecs: 300,
							},
						},
					}
					err := validateNewTask(ctx, req)
					assert.Loosely(t, err, should.ErrLike("when using secret_bytes multiple times, all values must match"))
				})
			})

			t.Run("expiration_secs", func(t *ftt.Test) {
				t.Run("too_short", func(t *ftt.Test) {
					req := &apipb.NewTaskRequest{
						Name: "new",
						TaskSlices: []*apipb.TaskSlice{
							{
								Properties: &apipb.TaskProperties{},
							},
						},
					}
					err := validateNewTask(ctx, req)
					assert.Loosely(t, err, should.ErrLike("must be between"))
				})

				t.Run("too_long", func(t *ftt.Test) {
					req := &apipb.NewTaskRequest{
						Name:                 "new",
						BotPingToleranceSecs: 300,
						TaskSlices: []*apipb.TaskSlice{
							{
								Properties:     &apipb.TaskProperties{},
								ExpirationSecs: maxExpirationSecs + 1,
							},
						},
					}
					err := validateNewTask(ctx, req)
					assert.Loosely(t, err, should.ErrLike("must be between"))
				})
			})

			t.Run("task_properties", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					req := &apipb.NewTaskRequest{
						Name: "new",
						TaskSlices: []*apipb.TaskSlice{
							{
								ExpirationSecs: 600,
							},
						},
					}
					err := validateNewTask(ctx, req)
					assert.Loosely(t, err, should.ErrLike("invalid properties of slice 0: required"))
				})
				t.Run("grace_period_secs", func(t *ftt.Test) {
					t.Run("too_short", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.GracePeriodSecs = 10
						err := validateNewTask(ctx, req)
						assert.Loosely(t, err, should.ErrLike("must be between"))
					})
					t.Run("too_long", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.GracePeriodSecs = maxGracePeriodSecs + 1
						err := validateNewTask(ctx, req)
						assert.Loosely(t, err, should.ErrLike("must be between"))
					})
				})

				t.Run("command", func(t *ftt.Test) {
					t.Run("empty", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Command = []string{}
						err := validateNewTask(ctx, req)
						assert.Loosely(t, err, should.ErrLike("command: required"))
					})
					t.Run("too_many_args", func(t *ftt.Test) {
						req := simpliestValidRequest()
						for i := 0; i < maxCmdArgs+1; i++ {
							req.TaskSlices[0].Properties.Command = append(req.TaskSlices[0].Properties.Command, "arg")
						}
						err := validateNewTask(ctx, req)
						assert.Loosely(t, err, should.ErrLike("can have up to 128 arguments"))
					})
				})

				t.Run("env", func(t *ftt.Test) {
					t.Run("too_many_keys", func(t *ftt.Test) {
						req := simpliestValidRequest()
						envItem := &apipb.StringPair{
							Key:   "key",
							Value: "value",
						}
						for i := 0; i < maxEnvKeyCount+1; i++ {
							req.TaskSlices[0].Properties.Env = append(req.TaskSlices[0].Properties.Env, envItem)
						}
						err := validateNewTask(ctx, req)
						assert.Loosely(t, err, should.ErrLike("can have up to 64 keys"))
					})

					t.Run("duplicate_keys", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Env = []*apipb.StringPair{
							{
								Key:   "key",
								Value: "value",
							},
							{
								Key:   "key",
								Value: "value",
							},
						}
						err := validateNewTask(ctx, req)
						assert.Loosely(t, err, should.ErrLike("same key cannot be specified twice"))
					})

					t.Run("key", func(t *ftt.Test) {
						t.Run("empty", func(t *ftt.Test) {
							req := simpliestValidRequest()
							req.TaskSlices[0].Properties.Env = []*apipb.StringPair{
								{
									Value: "value",
								},
							}
							err := validateNewTask(ctx, req)
							assert.Loosely(t, err, should.ErrLike("key is required"))
						})

						t.Run("too_long", func(t *ftt.Test) {
							t.Run("empty", func(t *ftt.Test) {
								req := simpliestValidRequest()
								req.TaskSlices[0].Properties.Env = []*apipb.StringPair{
									{
										Key:   strings.Repeat("a", maxEnvKeyLength+1),
										Value: "value",
									},
								}
								err := validateNewTask(ctx, req)
								assert.Loosely(t, err, should.ErrLike("too long"))
							})
						})

						t.Run("invalid", func(t *ftt.Test) {
							t.Run("empty", func(t *ftt.Test) {
								req := simpliestValidRequest()
								req.TaskSlices[0].Properties.Env = []*apipb.StringPair{
									{
										Key:   "1",
										Value: "value",
									},
								}
								err := validateNewTask(ctx, req)
								assert.Loosely(t, err, should.ErrLike("should match"))
							})
						})
					})

					t.Run("value_too_long", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Env = []*apipb.StringPair{
							{
								Key:   "key",
								Value: strings.Repeat("a", maxEnvValueLength+1),
							},
						}
						err := validateNewTask(ctx, req)
						assert.Loosely(t, err, should.ErrLike("too long"))
					})
				})

				t.Run("env_prefixes", func(t *ftt.Test) {
					t.Run("too_many_keys", func(t *ftt.Test) {
						req := simpliestValidRequest()
						epItem := &apipb.StringListPair{
							Key: "key",
							Value: []string{
								"value",
							},
						}
						for i := 0; i < maxEnvKeyCount+1; i++ {
							req.TaskSlices[0].Properties.EnvPrefixes = append(req.TaskSlices[0].Properties.EnvPrefixes, epItem)
						}
						err := validateNewTask(ctx, req)
						assert.Loosely(t, err, should.ErrLike("can have up to 64 keys"))
					})

					t.Run("duplicate_keys", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.EnvPrefixes = []*apipb.StringListPair{
							{
								Key:   "key",
								Value: []string{"value"},
							},
							{
								Key:   "key",
								Value: []string{"value"},
							},
						}
						err := validateNewTask(ctx, req)
						assert.Loosely(t, err, should.ErrLike("same key cannot be specified twice"))
					})

					t.Run("value", func(t *ftt.Test) {
						t.Run("empty", func(t *ftt.Test) {
							req := simpliestValidRequest()
							req.TaskSlices[0].Properties.EnvPrefixes = []*apipb.StringListPair{
								{
									Key: "key",
								},
							}
							err := validateNewTask(ctx, req)
							assert.Loosely(t, err, should.ErrLike("value is required"))
						})
						t.Run("with_invalid_path", func(t *ftt.Test) {
							cases := []struct {
								name string
								path string
								err  any
							}{
								{"empty", "", "cannot be empty"},
								{"too_long", strings.Repeat("a", maxEnvValueLength+1), "too long"},
								{"with_double_backslashes", "a\\b", `cannot contain "\\".`},
								{"with_leading_slash", "/a/b", `cannot start with "/"`},
								{"not_normalized_dot", "./a/b", "is not normalized"},
								{"not_normalized_double_dots", "a/../b", "is not normalized"},
							}
							for _, cs := range cases {
								t.Run(cs.name, func(t *ftt.Test) {
									req := simpliestValidRequest()
									req.TaskSlices[0].Properties.EnvPrefixes = []*apipb.StringListPair{
										{
											Key: "key",
											Value: []string{
												cs.path,
											},
										},
									}
									err := validateNewTask(ctx, req)
									assert.Loosely(t, err, should.ErrLike(cs.err))
								})
							}
						})
					})
				})

				t.Run("cas_input_root", func(t *ftt.Test) {
					t.Run("cas_instance", func(t *ftt.Test) {
						t.Run("empty", func(t *ftt.Test) {
							req := simpliestValidRequest()
							req.TaskSlices[0].Properties.CasInputRoot = &apipb.CASReference{}
							err := validateNewTask(ctx, req)
							assert.Loosely(t, err, should.ErrLike("cas_instance is required"))
						})
						t.Run("invalid", func(t *ftt.Test) {
							req := simpliestValidRequest()
							req.TaskSlices[0].Properties.CasInputRoot = &apipb.CASReference{
								CasInstance: "invalid",
							}
							err := validateNewTask(ctx, req)
							assert.Loosely(t, err, should.ErrLike("should match"))
						})
					})
					t.Run("digest", func(t *ftt.Test) {
						req := simpliestValidRequest()
						casInputRoot := &apipb.CASReference{
							CasInstance: "projects/project/instances/instance",
						}
						req.TaskSlices[0].Properties.CasInputRoot = casInputRoot
						t.Run("empty", func(t *ftt.Test) {
							err := validateNewTask(ctx, req)
							assert.Loosely(t, err, should.ErrLike("digest: required"))
						})
						t.Run("empty_hash", func(t *ftt.Test) {
							casInputRoot.Digest = &apipb.Digest{}
							err := validateNewTask(ctx, req)
							assert.Loosely(t, err, should.ErrLike("hash is required"))
						})
						t.Run("zero_size", func(t *ftt.Test) {
							casInputRoot.Digest = &apipb.Digest{
								Hash: "hash",
							}
							err := validateNewTask(ctx, req)
							assert.Loosely(t, err, should.ErrLike("size_bytes is required"))
						})
					})
				})

				t.Run("outputs", func(t *ftt.Test) {
					t.Run("too_many", func(t *ftt.Test) {
						req := simpliestValidRequest()
						for i := 0; i < maxOutputCount+1; i++ {
							req.TaskSlices[0].Properties.Outputs = append(req.TaskSlices[0].Properties.Outputs, "output")
						}
						err := validateNewTask(ctx, req)
						assert.Loosely(t, err, should.ErrLike("can have up to 512 outputs"))
					})
				})
			})
		})

		t.Run("OK", func(t *ftt.Test) {
			t.Run("without_optional_fields", func(t *ftt.Test) {
				req := simpliestValidRequest()
				err := validateNewTask(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})

			t.Run("with_all_fields", func(t *ftt.Test) {
				slice := func(secretBytes []byte) *apipb.TaskSlice {
					s := &apipb.TaskSlice{
						Properties: &apipb.TaskProperties{
							GracePeriodSecs:      60,
							ExecutionTimeoutSecs: 300,
							IoTimeoutSecs:        300,
							Command:              []string{"command", "arg"},
							Dimensions: []*apipb.StringPair{
								{
									Key:   "pool",
									Value: "pool",
								},
							},
						},
						ExpirationSecs: 300,
					}

					if len(secretBytes) > 0 {
						s.Properties.SecretBytes = secretBytes
					}

					return s
				}

				req := &apipb.NewTaskRequest{
					Name:                 "new",
					ParentTaskId:         "60b2ed0a43023111",
					Priority:             30,
					ServiceAccount:       "bot",
					PubsubTopic:          "projects/project/topics/topic",
					PubsubUserdata:       "userdata",
					BotPingToleranceSecs: 300,
					Realm:                "project:realm",
					Tags: []string{
						"k1:v1",
						"k2:v2",
					},
					TaskSlices: []*apipb.TaskSlice{
						slice([]byte("this is a secret")),
						slice(nil),
					},
				}
				err := validateNewTask(ctx, req)
				assert.Loosely(t, err, should.BeNil)
			})
		})
	})
}
