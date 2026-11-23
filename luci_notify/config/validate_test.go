// Copyright 2017 The LUCI Authors.
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

package config

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	tspb "go.chromium.org/luci/tree_status/proto/v1"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/common"
	"go.chromium.org/luci/luci_notify/testutil"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	ftt.Run(`Test Environment for validateProjectConfig`, t, func(t *ftt.Test) {
		testValidation := func(env, config, expectFormat string, expectArgs ...any) {
			t.Run(env, func(t *ftt.Test) {
				cfg, err := testutil.ParseProjectConfig(config)
				assert.Loosely(t, err, should.BeNil)
				ctx := &validation.Context{Context: context.Background()}
				assert.Loosely(t, validateProjectConfig(ctx, "test-project", cfg, nil), should.BeNil)
				err = ctx.Finalize()
				if expectFormat == "" {
					assert.Loosely(t, err, should.BeNil)
					return
				}
				expect := fmt.Sprintf(expectFormat, expectArgs...)
				assert.Loosely(t, err, should.ErrLike(expect))
			})
		}
		testValidation(`empty`, ``, "")

		testValidation(`builder missing name`, `
			notifiers {
				name: "good-name"
				builders {
					bucket: "test.bucket"
				}
			}`,
			requiredFieldError, "name")

		testValidation(`builder missing bucket`, `
			notifiers {
				name: "good-name"
				builders {
					name: "i-am-a-builder"
				}
			}`,
			requiredFieldError, "bucket")

		testValidation(`builder bad repo`, `
			notifiers {
				name: "good-name"
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
					repository: "bad://x.notgooglesource.com/+/hello"
				}
			}`,
			badRepoURLError, "bad://x.notgooglesource.com/+/hello")

		testValidation(`bad email address`, `
			notifiers {
				name: "good-name"
				notifications {
					on_new_status: SUCCESS
					on_new_status: FAILURE
					on_new_status: INFRA_FAILURE
					email {
						recipients: "@@@@@"
					}
				}
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
				}
			}`,
			badEmailError, "@@@@@")

		testValidation(`duplicate builders in notifier`, `
			notifiers {
				name: "good-name"
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
				}
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
				}
			}`,
			duplicateBuilderError, "test.bucket/i-am-a-builder")

		testValidation(`duplicate builders in project`, `
			notifiers {
				name: "good-name"
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
				}
			}
			notifiers {
				name: "good-name-again"
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
				}
			}`,
			duplicateBuilderError, "test.bucket/i-am-a-builder")

		testValidation(`different bucketname same builders OK`, `
			notifiers {
				name: "good-name"
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket"
				}
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket3"
				}
			}
			notifiers {
				name: "good-name-again"
				builders {
					name: "i-am-a-builder"
					bucket: "test.bucket2"
				}
			}`,
			"")

		testValidation(`bad failed_step_regexp in notification`, `
			notifiers {
				name: "invalid"
				notifications: {
					failed_step_regexp: "["
				}
			}`,
			badRegexError, "failed_step_regexp", "error parsing regexp: missing closing ]: `[`")

		testValidation(`bad failed_step_regexp_exclude in notification`, `
			notifiers {
				name: "invalid"
				notifications: {
					failed_step_regexp_exclude: "x{3,2}"
				}
			}`,
			badRegexError, "failed_step_regexp_exclude", "error parsing regexp: invalid repeat count: `{3,2}`")

		testValidation(`bad failed_step_regexp in tree_closer`, `
			notifiers {
				name: "invalid"
				tree_closers: {
					tree_name: "example"
					failed_step_regexp: ")"
				}
			}`,
			badRegexError, "failed_step_regexp", "error parsing regexp: unexpected ): `)`")

		testValidation(`bad failed_step_regexp_exclude in tree_closer`, `
			notifiers {
				name: "invalid"
				tree_closers: {
					tree_name: "example"
					failed_step_regexp_exclude: "[z-a]"
				}
			}`,
			badRegexError, "failed_step_regexp_exclude", "error parsing regexp: invalid character class range: `z-a`")

		testValidation(`missing tree_name in tree_closer`, `
			notifiers {
				name: "invalid"
				tree_closers {
					template: "foo"
				}
			}`,
			requiredFieldError, "tree_name")

		testValidation(`bad tree_name in tree_closer`, `
			notifiers {
				name: "invalid"
				tree_closers {
					tree_name: "bad.tree.name"
					template: "foo"
				}
			}`,
			invalidFieldError, "tree_name")

		testValidation(`bad tree_status_host in tree_closer`, `
			notifiers {
				name: "invalid"
				tree_closers {
					tree_status_host: "bad.tree.status.host"
					template: "foo"
				}
			}`,
			invalidFieldError, "tree_status_host")

		testValidation(`both tree_name and tree_status_host in tree_closer`, `
			notifiers {
				name: "invalid"
				tree_closers {
					tree_status_host: "myapp-status.appspot.com"
					template: "foo"
					tree_name: "myapp"
				}
			}`,
			onlyTreeNameOrHostError)

		testValidation(`duplicate tree_name within notifier`, `
			notifiers {
				name: "invalid"
				tree_closers {
					tree_name: "tree1"
				}
				tree_closers {
					tree_name: "tree1"
				}
			}`,
			duplicateTreeNameError, "tree1")

		testValidation(`duplicate tree_name within notifier (mixed tree_name and tree_status_host)`, `
			notifiers {
				name: "invalid"
				tree_closers {
					tree_name: "tree1"
				}
				tree_closers {
					tree_status_host: "tree1-status.appspot.com"
				}
			}`,
			duplicateTreeNameError, "tree1")

		testValidation(`duplicate tree_name, different notifiers`, `
			notifiers {
				name: "fine"
				tree_closers {
					tree_name: "tree1"
				}
			}
			notifiers {
				name: "also fine"
				tree_closers {
					tree_name: "tree1"
				}
			}`,
			"")
	})

	ftt.Run(`Test Environment for validateSettings`, t, func(t *ftt.Test) {
		testValidation := func(env, config, expectFormat string, expectArgs ...any) {
			t.Run(env, func(t *ftt.Test) {
				cfg, err := testutil.ParseSettings(config)
				assert.Loosely(t, err, should.BeNil)
				ctx := &validation.Context{Context: context.Background()}
				ctx.SetFile("settings.cfg")
				validateSettings(ctx, cfg)
				err = ctx.Finalize()
				if expectFormat == "" {
					assert.Loosely(t, err, should.BeNil)
					return
				}
				expect := fmt.Sprintf(expectFormat, expectArgs...)
				assert.Loosely(t, err, should.ErrLike(expect))
			})
		}
		testValidation(`bad hostname`, `luci_tree_status_host: "9mNRn29%^^%#"`, invalidFieldError, "luci_tree_status_host")
		testValidation(`good`, `luci_tree_status_host: "luci-tree-status.example.com"`, "")
	})

	ftt.Run("email template filename validation", t, func(t *ftt.Test) {
		c := common.SetAppIDForTest(context.Background(), "luci-notify")
		ctx := &validation.Context{Context: c}
		validFileContent := []byte("a\n\nb")

		t.Run("valid", func(t *ftt.Test) {
			validateEmailTemplateFile(ctx, "projects/x", "luci-notify/email-templates/a.template", validFileContent)
			assert.Loosely(t, ctx.Finalize(), should.BeNil)
		})

		t.Run("invalid char", func(t *ftt.Test) {
			validateEmailTemplateFile(ctx, "projects/x", "luci-notify/email-templates/A.template", validFileContent)
			assert.Loosely(t, ctx.Finalize(), should.ErrLike("does not match"))
		})
	})

	ftt.Run("Tree ownership validation in validateProjectConfigFile", t, func(t *ftt.Test) {
		trees := map[string]*tspb.Tree{
			"chromium": {
				Name:     "trees/chromium",
				Projects: []string{"chromium"},
			},
			"infra": {
				Name:     "trees/infra",
				Projects: []string{"infra", "infra-internal"},
			},
			"devtools": {
				Name:     "trees/devtools",
				Projects: []string{"devtools-frontend"},
			},
			"v8": {
				Name:     "trees/v8",
				Projects: []string{"v8", "v8-internal"},
			},
			"empty-tree": {
				Name: "trees/empty-tree",
			},
		}
		treeErrors := map[string]error{}
		callsByTree := map[string]int{}

		getter := func(ctx context.Context, treeName string) (*tspb.Tree, error) {
			callsByTree[treeName]++
			if err, ok := treeErrors[treeName]; ok {
				return nil, err
			}
			if tree, ok := trees[treeName]; ok {
				return tree, nil
			}
			return nil, grpcstatus.Errorf(codes.NotFound, "tree %q not found", treeName)
		}

		runValidate := func(configSet, cfgContent string) (opErr error, valErr error) {
			c := withTreeGetter(context.Background(), getter)
			vctx := &validation.Context{Context: c}
			vctx.SetFile("luci-notify.cfg")
			opErr = validateProjectConfigFile(vctx, configSet, "luci-notify.cfg", []byte(cfgContent))
			valErr = vctx.Finalize()
			return opErr, valErr
		}

		t.Run("authorized primary project", func(t *ftt.Test) {
			opErr, valErr := runValidate("projects/chromium", `
				notifiers {
					name: "chromium-closer"
					tree_closers {
						tree_name: "chromium"
					}
					builders {
						bucket: "ci"
						name: "linux"
					}
				}`)
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, valErr, should.BeNil)
		})

		t.Run("authorized secondary project on multi-project tree", func(t *ftt.Test) {
			opErr, valErr := runValidate("projects/infra-internal", `
				notifiers {
					name: "infra-internal-closer"
					tree_closers {
						tree_name: "infra"
					}
					builders {
						bucket: "ci"
						name: "linux"
					}
				}`)
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, valErr, should.BeNil)
		})

		t.Run("authorized project whose name differs from tree name", func(t *ftt.Test) {
			opErr, valErr := runValidate("projects/devtools-frontend", `
				notifiers {
					name: "devtools-closer"
					tree_closers {
						tree_name: "devtools"
					}
					builders {
						bucket: "ci"
						name: "linux"
					}
				}`)
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, valErr, should.BeNil)
		})

		t.Run("unauthorized cross-project tree_name rejected", func(t *ftt.Test) {
			opErr, valErr := runValidate("projects/hostile-x", `
				notifiers {
					name: "hostile-closer"
					tree_closers {
						tree_name: "chromium"
					}
					builders {
						bucket: "ci"
						name: "always-fail"
					}
				}`)
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, valErr, should.ErrLike(`project "hostile-x" is not authorized to close tree "chromium"; check the tree's "projects" list in the LUCI Tree Status service config (config.cfg)`))
		})

		t.Run("unauthorized cross-project legacy tree_status_host rejected", func(t *ftt.Test) {
			opErr, valErr := runValidate("projects/hostile-x", `
				notifiers {
					name: "hostile-closer"
					tree_closers {
						tree_status_host: "chromium-status.appspot.com"
					}
					builders {
						bucket: "ci"
						name: "always-fail"
					}
				}`)
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, valErr, should.ErrLike(`project "hostile-x" is not authorized to close tree "chromium"`))
		})

		t.Run("tree with empty projects list rejects all projects", func(t *ftt.Test) {
			opErr, valErr := runValidate("projects/chromium", `
				notifiers {
					name: "empty-tree-closer"
					tree_closers {
						tree_name: "empty-tree"
					}
					builders {
						bucket: "ci"
						name: "linux"
					}
				}`)
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, valErr, should.ErrLike(`project "chromium" is not authorized to close tree "empty-tree"`))
		})

		t.Run("unknown tree rejected", func(t *ftt.Test) {
			opErr, valErr := runValidate("projects/chromium", `
				notifiers {
					name: "unknown-tree-closer"
					tree_closers {
						tree_name: "nonexistent"
					}
					builders {
						bucket: "ci"
						name: "linux"
					}
				}`)
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, valErr, should.ErrLike(`tree "nonexistent" is not configured in LUCI Tree Status`))
		})

		t.Run("permission denied tree rejected", func(t *ftt.Test) {
			treeErrors["secret-tree"] = grpcstatus.Errorf(codes.PermissionDenied, "access denied")
			opErr, valErr := runValidate("projects/chromium", `
				notifiers {
					name: "secret-tree-closer"
					tree_closers {
						tree_name: "secret-tree"
					}
					builders {
						bucket: "ci"
						name: "linux"
					}
				}`)
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, valErr, should.ErrLike(`LUCI Notify does not have access to tree "secret-tree" in LUCI Tree Status`))
			assert.Loosely(t, valErr, should.ErrLike(`role/treestatus.reader (or role/treestatus.limitedReader) in the tree's realms.cfg`))
			assert.Loosely(t, valErr, should.ErrLike(`land realms.cfg before luci-notify.cfg`))
			assert.Loosely(t, valErr, should.ErrLike(`luci-tree-status-access`))
		})

		t.Run("memoizes GetTree lookups across notifiers in same config", func(t *ftt.Test) {
			opErr, valErr := runValidate("projects/v8", `
				notifiers {
					name: "closer-1"
					tree_closers {
						tree_name: "v8"
					}
					builders {
						bucket: "ci"
						name: "linux-1"
					}
				}
				notifiers {
					name: "closer-2"
					tree_closers {
						tree_name: "v8"
					}
					builders {
						bucket: "ci"
						name: "linux-2"
					}
				}
				notifiers {
					name: "closer-3"
					tree_closers {
						tree_name: "v8"
					}
					builders {
						bucket: "ci"
						name: "linux-3"
					}
				}`)
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, valErr, should.BeNil)
			assert.Loosely(t, callsByTree["v8"], should.Equal(1))
		})

		t.Run("does not fetch trees when both tree_name and tree_status_host are set", func(t *ftt.Test) {
			opErr, valErr := runValidate("projects/chromium", `
				notifiers {
					name: "ambiguous-closer"
					tree_closers {
						tree_name: "nonexistent"
						tree_status_host: "v8-status.appspot.com"
					}
					builders {
						bucket: "ci"
						name: "linux"
					}
				}`)
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, valErr, should.ErrLike(onlyTreeNameOrHostError))
			assert.Loosely(t, valErr.Error(), should.NotContainSubstring("not configured in LUCI Tree Status"))
			assert.Loosely(t, len(callsByTree), should.Equal(0))
		})

		t.Run("does not fetch settings or trees when no tree_closers configured", func(t *ftt.Test) {
			// Context without treeGetter or Datastore settings still succeeds when
			// there are no tree_closers.
			vctx := &validation.Context{Context: context.Background()}
			vctx.SetFile("luci-notify.cfg")
			opErr := validateProjectConfigFile(vctx, "projects/chromium", "luci-notify.cfg", []byte(`
				notifiers {
					name: "email-only"
					notifications {
						email {
							recipients: "dev@chromium.org"
						}
					}
					builders {
						bucket: "ci"
						name: "linux"
					}
				}`))
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
			assert.Loosely(t, len(callsByTree), should.Equal(0))
		})

		t.Run("transient RPC error propagates as operational error for retry", func(t *ftt.Test) {
			treeErrors["chromium"] = grpcstatus.Errorf(codes.Unavailable, "backend unavailable")
			opErr, valErr := runValidate("projects/chromium", `
				notifiers {
					name: "chromium-closer"
					tree_closers {
						tree_name: "chromium"
					}
					builders {
						bucket: "ci"
						name: "linux"
					}
				}`)
			assert.Loosely(t, opErr, should.ErrLike(`validating tree "chromium"`))
			assert.Loosely(t, valErr, should.BeNil)
		})

		t.Run("non-project config set rejected", func(t *ftt.Test) {
			opErr, valErr := runValidate("services/luci-notify", ``)
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, valErr, should.ErrLike(`invalid project config set "services/luci-notify"`))
		})

		t.Run("missing service settings fails closed when tree_closers present", func(t *ftt.Test) {
			c := memory.Use(context.Background())
			vctx := &validation.Context{Context: c}
			vctx.SetFile("luci-notify.cfg")
			opErr := validateProjectConfigFile(vctx, "projects/chromium", "luci-notify.cfg", []byte(`
				notifiers {
					name: "chromium-closer"
					tree_closers {
						tree_name: "chromium"
					}
					builders {
						bucket: "ci"
						name: "linux"
					}
				}`))
			assert.Loosely(t, opErr, should.ErrLike("fetching service settings for tree validation"))
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("empty luci_tree_status_host fails closed when tree_closers present", func(t *ftt.Test) {
			c := memory.Use(context.Background())
			assert.Loosely(t, datastore.Put(c, &Settings{
				Revision: "rev1",
				Settings: notifypb.Settings{},
			}), should.BeNil)

			vctx := &validation.Context{Context: c}
			vctx.SetFile("luci-notify.cfg")
			opErr := validateProjectConfigFile(vctx, "projects/chromium", "luci-notify.cfg", []byte(`
				notifiers {
					name: "chromium-closer"
					tree_closers {
						tree_name: "chromium"
					}
					builders {
						bucket: "ci"
						name: "linux"
					}
				}`))
			assert.Loosely(t, opErr, should.ErrLike("luci_tree_status_host is not configured in settings.cfg"))
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("end-to-end pRPC tree lookup via Settings in datastore", func(t *ftt.Test) {
			r := router.New()
			prpcSrv := &prpc.Server{}
			tspb.RegisterTreesServer(prpcSrv, &fakeTreesServer{
				trees: trees,
			})
			prpcSrv.InstallHandlers(r, nil)
			httpSrv := httptest.NewServer(r)
			defer httpSrv.Close()

			c := memory.Use(context.Background())
			c = authtest.MockAuthConfig(c)
			host := strings.TrimPrefix(httpSrv.URL, "http://")
			assert.Loosely(t, datastore.Put(c, &Settings{
				Revision: "rev1",
				Settings: notifypb.Settings{
					LuciTreeStatusHost: host,
				},
			}), should.BeNil)

			vctx := &validation.Context{Context: c}
			vctx.SetFile("luci-notify.cfg")
			opErr := validateProjectConfigFile(vctx, "projects/hostile-x", "luci-notify.cfg", []byte(`
				notifiers {
					name: "hostile-closer"
					tree_closers {
						tree_name: "chromium"
					}
					builders {
						bucket: "ci"
						name: "always-fail"
					}
				}`))
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`project "hostile-x" is not authorized to close tree "chromium"`))

			vctxOK := &validation.Context{Context: c}
			vctxOK.SetFile("luci-notify.cfg")
			opErr = validateProjectConfigFile(vctxOK, "projects/chromium", "luci-notify.cfg", []byte(`
				notifiers {
					name: "chromium-closer"
					tree_closers {
						tree_name: "chromium"
					}
					builders {
						bucket: "ci"
						name: "linux"
					}
				}`))
			assert.Loosely(t, opErr, should.BeNil)
			assert.Loosely(t, vctxOK.Finalize(), should.BeNil)
		})
	})
}

type fakeTreesServer struct {
	tspb.UnimplementedTreesServer
	trees map[string]*tspb.Tree
}

func (s *fakeTreesServer) GetTree(ctx context.Context, req *tspb.GetTreeRequest) (*tspb.Tree, error) {
	treeName := strings.TrimPrefix(req.GetName(), "trees/")
	if tree, ok := s.trees[treeName]; ok {
		return tree, nil
	}
	return nil, grpcstatus.Errorf(codes.NotFound, "tree %q not found", req.GetName())
}
