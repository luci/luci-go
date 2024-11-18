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
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

func simpliestValidSlice(pool string) *apipb.TaskSlice {
	return &apipb.TaskSlice{
		Properties: &apipb.TaskProperties{
			GracePeriodSecs:      60,
			ExecutionTimeoutSecs: 300,
			IoTimeoutSecs:        300,
			Command:              []string{"command", "arg"},
			Dimensions: []*apipb.StringPair{
				{
					Key:   "pool",
					Value: pool,
				},
			},
		},
		ExpirationSecs: 300,
	}
}

func simpliestValidRequest() *apipb.NewTaskRequest {
	return &apipb.NewTaskRequest{
		Name:     "new",
		Priority: 40,
		TaskSlices: []*apipb.TaskSlice{
			simpliestValidSlice("pool"),
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
			assert.That(t, err, should.ErrLike("name is required"))
		})

		t.Run("with_properties", func(t *ftt.Test) {
			req := &apipb.NewTaskRequest{
				Name:       "new",
				Properties: &apipb.TaskProperties{},
			}
			err := validateNewTask(ctx, req)
			assert.That(t, err, should.ErrLike("properties is deprecated"))
		})

		t.Run("with_expiration_secs", func(t *ftt.Test) {
			req := &apipb.NewTaskRequest{
				Name:           "new",
				ExpirationSecs: 300,
			}
			err := validateNewTask(ctx, req)
			assert.That(t, err, should.ErrLike("expiration_secs is deprecated"))
		})

		t.Run("parent_task_id", func(t *ftt.Test) {
			req := simpliestValidRequest()
			req.ParentTaskId = "invalid"
			err := validateNewTask(ctx, req)
			assert.That(t, err, should.ErrLike("bad task ID"))
		})

		t.Run("priority", func(t *ftt.Test) {
			t.Run("too_small", func(t *ftt.Test) {
				req := simpliestValidRequest()
				req.Priority = -1
				err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("must be between 0 and 255"))
			})
			t.Run("too_big", func(t *ftt.Test) {
				req := simpliestValidRequest()
				req.Priority = 500
				err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("must be between 0 and 255"))
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
					err := validateNewTask(ctx, req)
					assert.That(t, err, should.ErrLike(cs.err))
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
				{"invalid", "invalid", "does not match"},
			}
			for _, cs := range cases {
				t.Run(cs.name, func(t *ftt.Test) {
					req := simpliestValidRequest()
					req.PubsubTopic = cs.topic
					err := validateNewTask(ctx, req)
					assert.That(t, err, should.ErrLike(cs.err))
				})
			}
		})

		t.Run("pubsub_auth_token", func(t *ftt.Test) {
			t.Run("no topic", func(t *ftt.Test) {
				req := simpliestValidRequest()
				req.PubsubAuthToken = "token"
				err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("pubsub_auth_token requires pubsub_topic"))
			})

		})

		t.Run("pubsub_userdata", func(t *ftt.Test) {
			t.Run("no topic", func(t *ftt.Test) {
				req := simpliestValidRequest()
				req.PubsubUserdata = "data"
				err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("pubsub_userdata requires pubsub_topic"))
			})
			t.Run("too_long", func(t *ftt.Test) {
				req := simpliestValidRequest()
				req.PubsubTopic = "projects/project/topics/topic"
				req.PubsubUserdata = strings.Repeat("l", maxPubsubUserDataLength+1)
				err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("too long"))
			})

		})

		t.Run("bot_ping_tolerance", func(t *ftt.Test) {
			t.Run("too_small", func(t *ftt.Test) {
				req := simpliestValidRequest()
				req.BotPingToleranceSecs = 20
				err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("must be between"))
			})
			t.Run("too_big", func(t *ftt.Test) {
				req := simpliestValidRequest()
				req.BotPingToleranceSecs = 5000
				err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("must be between"))
			})
		})

		t.Run("realm", func(t *ftt.Test) {
			req := simpliestValidRequest()
			req.Realm = "invalid"
			err := validateNewTask(ctx, req)
			assert.That(t, err, should.ErrLike("invalid"))
		})

		t.Run("tags", func(t *ftt.Test) {
			t.Run("too_many", func(t *ftt.Test) {
				req := simpliestValidRequest()
				for i := 0; i < maxTagCount+1; i++ {
					req.Tags = append(req.Tags, "k:v")
				}
				err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("up to 256 tags"))
			})
			t.Run("invalid", func(t *ftt.Test) {
				req := simpliestValidRequest()
				req.Tags = []string{"invalid"}
				err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("invalid"))
			})
		})

		t.Run("task_slices", func(t *ftt.Test) {
			t.Run("no_task_slices", func(t *ftt.Test) {
				req := &apipb.NewTaskRequest{
					Name: "new",
				}
				err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("task_slices is required"))
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
					assert.That(t, err, should.ErrLike("exceeding limit"))
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
					assert.That(t, err, should.ErrLike("when using secret_bytes multiple times, all values must match"))
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
					assert.That(t, err, should.ErrLike("must be between"))
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
					assert.That(t, err, should.ErrLike("must be between"))
				})
			})

			t.Run("different_pools", func(t *ftt.Test) {
				req := &apipb.NewTaskRequest{
					Name:                 "new",
					BotPingToleranceSecs: 300,
					TaskSlices: []*apipb.TaskSlice{
						simpliestValidSlice("pool1"),
						simpliestValidSlice("pool2"),
					},
				}
				err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("each task slice must use the same pool dimensions"))
			})

			t.Run("duplicated_slices", func(t *ftt.Test) {
				req := &apipb.NewTaskRequest{
					Name:                 "new",
					BotPingToleranceSecs: 300,
					TaskSlices: []*apipb.TaskSlice{
						simpliestValidSlice("pool1"),
						simpliestValidSlice("pool1"),
					},
				}
				err := validateNewTask(ctx, req)
				assert.That(t, err, should.ErrLike("cannot request duplicate task slice"))
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
					assert.That(t, err, should.ErrLike("invalid properties of slice 0: required"))
				})
				t.Run("grace_period_secs", func(t *ftt.Test) {
					t.Run("too_short", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.GracePeriodSecs = 10
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("must be between"))
					})
					t.Run("too_long", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.GracePeriodSecs = maxGracePeriodSecs + 1
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("must be between"))
					})
				})

				t.Run("command", func(t *ftt.Test) {
					t.Run("empty", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Command = []string{}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("command: required"))
					})
					t.Run("too_many_args", func(t *ftt.Test) {
						req := simpliestValidRequest()
						for i := 0; i < maxCmdArgs+1; i++ {
							req.TaskSlices[0].Properties.Command = append(req.TaskSlices[0].Properties.Command, "arg")
						}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("can have up to 128 arguments"))
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
						assert.That(t, err, should.ErrLike("can have up to 64 keys"))
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
						assert.That(t, err, should.ErrLike("same key cannot be specified twice"))
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
							assert.That(t, err, should.ErrLike("key is required"))
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
						assert.That(t, err, should.ErrLike("too long"))
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
						assert.That(t, err, should.ErrLike("can have up to 64 keys"))
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
						assert.That(t, err, should.ErrLike("same key cannot be specified twice"))
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
							assert.That(t, err, should.ErrLike("value is required"))
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
							assert.That(t, err, should.ErrLike("cas_instance is required"))
						})
						t.Run("invalid", func(t *ftt.Test) {
							req := simpliestValidRequest()
							req.TaskSlices[0].Properties.CasInputRoot = &apipb.CASReference{
								CasInstance: "invalid",
							}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("should match"))
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
							assert.That(t, err, should.ErrLike("digest: required"))
						})
						t.Run("empty_hash", func(t *ftt.Test) {
							casInputRoot.Digest = &apipb.Digest{}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("hash is required"))
						})
						t.Run("zero_size", func(t *ftt.Test) {
							casInputRoot.Digest = &apipb.Digest{
								Hash: "hash",
							}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("size_bytes is required"))
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
						assert.That(t, err, should.ErrLike("can have up to 512 outputs"))
					})
				})

				t.Run("caches", func(t *ftt.Test) {
					t.Run("too_many", func(t *ftt.Test) {
						req := simpliestValidRequest()
						cache := &apipb.CacheEntry{
							Name: "name",
							Path: "path",
						}
						for i := 0; i < maxCacheCount+1; i++ {
							req.TaskSlices[0].Properties.Caches = append(req.TaskSlices[0].Properties.Caches, cache)
						}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("can have up to 32 caches"))
					})

					t.Run("name", func(t *ftt.Test) {
						t.Run("invalid", func(t *ftt.Test) {
							cases := []struct {
								tn   string
								name string
								err  any
							}{
								{"empty", "", "required"},
								{"too_long", strings.Repeat("a", maxCacheNameLength+1), "too long"},
								{"invalid", "INVALID", "should match"},
							}
							for _, cs := range cases {
								t.Run(cs.tn, func(t *ftt.Test) {
									req := simpliestValidRequest()
									req.TaskSlices[0].Properties.Caches = []*apipb.CacheEntry{
										{
											Name: cs.name,
										},
									}
									err := validateNewTask(ctx, req)
									assert.Loosely(t, err, should.ErrLike(cs.err))
								})
							}
							req := simpliestValidRequest()
							req.TaskSlices[0].Properties.Caches = []*apipb.CacheEntry{
								{
									Path: "path",
								},
							}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("cache name 0: required"))
						})
						t.Run("duplicates", func(t *ftt.Test) {
							req := simpliestValidRequest()
							req.TaskSlices[0].Properties.Caches = []*apipb.CacheEntry{
								{
									Name: "name",
									Path: "path",
								},
								{
									Name: "name",
									Path: "path",
								},
							}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("same cache name cannot be specified twice"))
						})
					})
					t.Run("path", func(t *ftt.Test) {
						// path validation is covered by env_prefixes
						t.Run("duplicates", func(t *ftt.Test) {
							req := simpliestValidRequest()
							req.TaskSlices[0].Properties.Caches = []*apipb.CacheEntry{
								{
									Name: "name1",
									Path: "a/b",
								},
								{
									Name: "name2",
									Path: "a/b",
								},
							}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("same cache path cannot be specified twice"))
						})
					})
				})

				t.Run("cipd_input", func(t *ftt.Test) {
					t.Run("server", func(t *ftt.Test) {
						cases := []struct {
							tn     string
							server string
							err    any
						}{
							{"empty", "", "required"},
							{"too_long", strings.Repeat("a", maxCIPDServerLength+1), "too long"},
						}
						for _, cs := range cases {
							t.Run(cs.tn, func(t *ftt.Test) {
								req := simpliestValidRequest()
								req.TaskSlices[0].Properties.CipdInput = &apipb.CipdInput{
									Server: cs.server,
								}
								err := validateNewTask(ctx, req)
								assert.That(t, err, should.ErrLike(cs.err))
							})
						}
					})
					t.Run("client_package", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.CipdInput = &apipb.CipdInput{
							Server: "https://cipd.com",
						}
						t.Run("nil", func(t *ftt.Test) {
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("required"))
						})
						t.Run("with_path", func(t *ftt.Test) {
							req.TaskSlices[0].Properties.CipdInput.ClientPackage = &apipb.CipdPackage{
								Path: "a/b",
							}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("path must be unset"))
						})
						t.Run("no name", func(t *ftt.Test) {
							req.TaskSlices[0].Properties.CipdInput.ClientPackage = &apipb.CipdPackage{}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("package_name is required"))
						})
						t.Run("invalid name", func(t *ftt.Test) {
							req.TaskSlices[0].Properties.CipdInput.ClientPackage = &apipb.CipdPackage{
								PackageName: ".",
							}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("invalid package name"))
						})
						t.Run("no_version", func(t *ftt.Test) {
							req.TaskSlices[0].Properties.CipdInput.ClientPackage = &apipb.CipdPackage{
								PackageName: "some/pkg",
							}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("version is required"))
						})
					})
					t.Run("packages", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.CipdInput = &apipb.CipdInput{
							Server: "https://cipd.com",
							ClientPackage: &apipb.CipdPackage{
								PackageName: "some/pkg",
								Version:     "version",
							},
						}
						t.Run("empty", func(t *ftt.Test) {
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("cannot have an empty package list"))
						})

						t.Run("too_many", func(t *ftt.Test) {
							for i := 0; i < maxCIPDPackageCount+1; i++ {
								req.TaskSlices[0].Properties.CipdInput.Packages = append(
									req.TaskSlices[0].Properties.CipdInput.Packages, &apipb.CipdPackage{})
							}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("up to 64 CIPD packages can be listed for a task"))
						})
						t.Run("duplicate", func(t *ftt.Test) {
							req.TaskSlices[0].Properties.CipdInput.Packages = []*apipb.CipdPackage{
								{
									PackageName: "some/pkg",
									Version:     "version1",
									Path:        "a/b",
								},
								{
									PackageName: "some/pkg",
									Version:     "version2",
									Path:        "a/b",
								},
							}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("specified more than once"))
						})
						t.Run("no_path", func(t *ftt.Test) {
							req.TaskSlices[0].Properties.CipdInput.Packages = []*apipb.CipdPackage{
								{
									PackageName: "some/pkg",
									Version:     "version",
								},
							}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("path: cannot be empty"))
						})
						t.Run("cache_path", func(t *ftt.Test) {
							req.TaskSlices[0].Properties.CipdInput.Packages = []*apipb.CipdPackage{
								{
									PackageName: "some/pkg",
									Version:     "version1",
									Path:        "a/b",
								},
							}
							req.TaskSlices[0].Properties.Caches = []*apipb.CacheEntry{
								{
									Name: "name",
									Path: "a/b",
								},
							}
							err := validateNewTask(ctx, req)
							assert.That(t, err, should.ErrLike("mapped to a named cache"))
						})
						t.Run("idempotent", func(t *ftt.Test) {
							t.Run("fail", func(t *ftt.Test) {
								req.TaskSlices[0].Properties.CipdInput.Packages = []*apipb.CipdPackage{
									{
										PackageName: "some/pkg",
										Version:     "version1",
										Path:        "a/b",
									},
								}
								req.TaskSlices[0].Properties.Idempotent = true
								err := validateNewTask(ctx, req)
								assert.That(t, err, should.ErrLike("cannot have unpinned packages"))
							})
							t.Run("pass_tag", func(t *ftt.Test) {
								req.TaskSlices[0].Properties.CipdInput.Packages = []*apipb.CipdPackage{
									{
										PackageName: "some/pkg",
										Version:     "good:tag",
										Path:        "a/b",
									},
								}
								req.TaskSlices[0].Properties.Idempotent = true
								err := validateNewTask(ctx, req)
								assert.That(t, err, should.ErrLike(nil))
							})
							t.Run("pass_hash", func(t *ftt.Test) {
								req.TaskSlices[0].Properties.CipdInput.Packages = []*apipb.CipdPackage{
									{
										PackageName: "some/pkg",
										Version:     "B7r75joOfFfFcq7fHCKAIrU34oeFAT174Bf8eHMajMUC",
										Path:        "a/b",
									},
								}
								req.TaskSlices[0].Properties.Idempotent = true
								err := validateNewTask(ctx, req)
								assert.That(t, err, should.ErrLike(nil))
							})
						})
					})
				})

				t.Run("dimensions", func(t *ftt.Test) {
					t.Run("too_many_dimensions", func(t *ftt.Test) {
						req := simpliestValidRequest()
						for i := 0; i < maxDimensionKeyCount+1; i++ {
							req.TaskSlices[0].Properties.Dimensions = append(
								req.TaskSlices[0].Properties.Dimensions, &apipb.StringPair{
									Key:   fmt.Sprintf("key%d", i),
									Value: "value",
								})
						}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("can have up to 32 keys"))
					})
					t.Run("invalid_key", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "bad key",
								Value: "value",
							},
						}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("should match"))
					})

					t.Run("no_pool", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "key",
								Value: "value",
							},
						}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("pool is required"))
					})
					t.Run("multiple_pool", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "pool",
								Value: "p1",
							},
							{
								Key:   "pool",
								Value: "p2",
							},
						}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("pool cannot be specified more than once"))
					})
					t.Run("multiple_pool_with_ordim", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "pool",
								Value: "p1|p2",
							},
						}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("pool cannot be specified more than once"))
					})
					t.Run("multiple_id", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "id",
								Value: "id1",
							},
							{
								Key:   "id",
								Value: "id2",
							},
						}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("id cannot be specified more than once"))
					})
					t.Run("too_many_dimension_values", func(t *ftt.Test) {
						req := simpliestValidRequest()
						for i := 0; i < maxDimensionValueCount+1; i++ {
							req.TaskSlices[0].Properties.Dimensions = append(
								req.TaskSlices[0].Properties.Dimensions, &apipb.StringPair{
									Key:   "key",
									Value: "value",
								})
						}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("can have up to 16 values for key"))
					})
					t.Run("repeated_values", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
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
						assert.That(t, err, should.ErrLike("repeated values"))
					})
					t.Run("invalid_value", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "key",
								Value: " bad value",
							},
						}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("no leading or trailing spaces"))
					})
					t.Run("invalid_or_values", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "key",
								Value: "v1| v2",
							},
						}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("no leading or trailing spaces"))
					})
					t.Run("too_many_or_values", func(t *ftt.Test) {
						req := simpliestValidRequest()
						req.TaskSlices[0].Properties.Dimensions = []*apipb.StringPair{
							{
								Key:   "key1",
								Value: "v1|v2|v3",
							},
							{
								Key:   "key2",
								Value: "v4|v5|v6",
							},
						}
						err := validateNewTask(ctx, req)
						assert.That(t, err, should.ErrLike("or dimensions should not be more than 8"))
					})
				})
			})
		})

		t.Run("OK", func(t *ftt.Test) {
			t.Run("without_optional_fields", func(t *ftt.Test) {
				req := simpliestValidRequest()
				assert.That(t, validateNewTask(ctx, req), should.ErrLike(nil))
			})

			t.Run("with_all_fields", func(t *ftt.Test) {
				slice := func(secretBytes []byte) *apipb.TaskSlice {
					s := &apipb.TaskSlice{
						Properties: &apipb.TaskProperties{
							GracePeriodSecs:      60,
							ExecutionTimeoutSecs: 300,
							IoTimeoutSecs:        300,
							Idempotent:           true,
							Command:              []string{"command", "arg"},
							RelativeCwd:          "cwd",
							Env: []*apipb.StringPair{
								{
									Key:   "key",
									Value: "value",
								},
							},
							EnvPrefixes: []*apipb.StringListPair{
								{
									Key: "key",
									Value: []string{
										"a/b",
									},
								},
							},
							CasInputRoot: &apipb.CASReference{
								CasInstance: "projects/project/instances/instance",
								Digest: &apipb.Digest{
									Hash:      "hash",
									SizeBytes: 1024,
								},
							},
							Outputs: []string{
								"output",
							},
							Caches: []*apipb.CacheEntry{
								{
									Name: "name",
									Path: "path",
								},
							},
							CipdInput: &apipb.CipdInput{
								Server: "https://cipd.com",
								ClientPackage: &apipb.CipdPackage{
									PackageName: "some/pkg",
									Version:     "version",
								},
								Packages: []*apipb.CipdPackage{
									{
										PackageName: "some/pkg",
										Version:     "good:tag",
										Path:        "c/d",
									},
								},
							},
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
					PubsubAuthToken:      "token",
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
					Resultdb: &apipb.ResultDBCfg{
						Enable: true,
					},
					PoolTaskTemplate: apipb.NewTaskRequest_AUTO,
				}
				assert.That(t, validateNewTask(ctx, req), should.ErrLike(nil))
			})
		})
	})
}
