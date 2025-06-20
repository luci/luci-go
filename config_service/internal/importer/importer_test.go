// Copyright 2023 The LUCI Authors.
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

package importer

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/golang/mock/gomock"
	"github.com/julienschmidt/httprouter"
	"github.com/klauspost/compress/gzip"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	protoutil "go.chromium.org/luci/common/proto"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/gitiles/mock_gitiles"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/metrics"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/internal/settings"
	"go.chromium.org/luci/config_service/internal/taskpb"
	"go.chromium.org/luci/config_service/internal/validation"
	"go.chromium.org/luci/config_service/testutil"
)

func TestImportAllConfigs(t *testing.T) {
	t.Parallel()

	ftt.Run("import all configs", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		disp := &tq.Dispatcher{}
		ctx, sch := tq.TestingContext(ctx, disp)
		ctx = settings.WithGlobalConfigLoc(ctx, &cfgcommonpb.GitilesLocation{
			Repo: "https://a.googlesource.com/infradata/config",
			Ref:  "refs/heads/main",
			Path: "dev-configs",
		})

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := mock_gitiles.NewMockGitilesClient(ctl)
		ctx = context.WithValue(ctx, &clients.MockGitilesClientKey, mockClient)
		importer := &Importer{}
		importer.registerTQTask(disp)

		t.Run("ok", func(t *ftt.Test) {
			expectCall := func() {
				mockClient.EXPECT().ListFiles(gomock.Any(), protoutil.MatcherEqual(
					&gitilespb.ListFilesRequest{
						Project:    "infradata/config",
						Committish: "refs/heads/main",
						Path:       "dev-configs",
					},
				)).Return(&gitilespb.ListFilesResponse{
					Files: []*git.File{
						{
							Id:   "hash1",
							Path: "service1",
							Type: git.File_TREE,
						},
						{
							Id:   "hash2",
							Path: "service2",
							Type: git.File_TREE,
						},
						{
							Id:   "hash3",
							Path: "file1",
							Type: git.File_BLOB,
						},
					},
				}, nil)
			}

			t.Run("projects.cfg File entity not exist", func(t *ftt.Test) {
				expectCall()
				err := importAllConfigs(ctx, disp)
				assert.Loosely(t, err, should.BeNil)
				cfgSetsInQueue := getCfgSetsInTaskQueue(sch)
				assert.Loosely(t, cfgSetsInQueue, should.Resemble([]string{"services/service1", "services/service2"}))
			})

			t.Run("projects.cfg File entity exist", func(t *ftt.Test) {
				expectCall()
				testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
					"projects.cfg": &cfgcommonpb.ProjectsCfg{
						Projects: []*cfgcommonpb.Project{
							{Id: "proj1"},
						},
					},
				})
				err := importAllConfigs(ctx, disp)
				assert.Loosely(t, err, should.BeNil)
				cfgSetsInQueue := getCfgSetsInTaskQueue(sch)
				assert.Loosely(t, cfgSetsInQueue, should.Resemble([]string{"projects/proj1", "services/service1", "services/service2"}))
			})

			t.Run("delete stale config set", func(t *ftt.Test) {
				expectCall()
				stale := &model.ConfigSet{ID: config.MustServiceSet("stale")}
				assert.Loosely(t, datastore.Put(ctx, stale), should.BeNil)
				err := importAllConfigs(ctx, disp)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, stale), should.Equal(datastore.ErrNoSuchEntity))
			})
		})

		t.Run("error", func(t *ftt.Test) {
			t.Run("gitiles", func(t *ftt.Test) {
				mockClient.EXPECT().ListFiles(gomock.Any(), gomock.Any()).Return(nil, errors.New("gitiles error"))
				err := importAllConfigs(ctx, disp)
				assert.Loosely(t, err, should.ErrLike("failed to load service config sets: failed to call Gitiles to list files: gitiles error"))
			})

			t.Run("bad projects.cfg content", func(t *ftt.Test) {
				mockClient.EXPECT().ListFiles(gomock.Any(), gomock.Any()).Return(&gitilespb.ListFilesResponse{}, nil)
				testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
					"projects.cfg": &cfgcommonpb.ServicesCfg{
						Services: []*cfgcommonpb.Service{
							{Id: "my-service"},
						}}, // bad type
				})
				assert.Loosely(t, importAllConfigs(ctx, disp), should.ErrLike(`failed to load project config sets: failed to unmarshal file "projects.cfg": proto`))
			})
		})
	})
}

type mockValidator struct {
	result *cfgcommonpb.ValidationResult
	err    error
}

func (mv *mockValidator) Validate(context.Context, config.Set, []validation.File) (*cfgcommonpb.ValidationResult, error) {
	return mv.result, mv.err
}

func TestImportConfigSet(t *testing.T) {
	t.Parallel()

	ftt.Run("import single ConfigSet", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		ctx = settings.WithGlobalConfigLoc(ctx, &cfgcommonpb.GitilesLocation{
			Repo: "https://a.googlesource.com/infradata/config",
			Ref:  "refs/heads/main",
			Path: "dev-configs",
		})
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		ctl := gomock.NewController(t)
		mockGtClient := mock_gitiles.NewMockGitilesClient(ctl)
		ctx = context.WithValue(ctx, &clients.MockGitilesClientKey, mockGtClient)
		mockGsClient := clients.NewMockGsClient(ctl)
		ctx = clients.WithGsClient(ctx, mockGsClient)
		const testGSBucket = "test-bucket"
		cs := config.MustServiceSet("my-service")
		latestCommit := &git.Commit{
			Id: "latest revision",
			Committer: &git.Commit_User{
				Name:  "user",
				Email: "user@gmail.com",
				Time:  timestamppb.New(datastore.RoundTime(clock.Now(ctx).UTC())),
			},
			Author: &git.Commit_User{
				Email: "author@gmail.com",
			},
		}
		expectedLatestRevInfo := model.RevisionInfo{
			ID:             latestCommit.Id,
			CommitTime:     latestCommit.Committer.Time.AsTime(),
			CommitterEmail: latestCommit.Committer.Email,
			AuthorEmail:    latestCommit.Author.Email,
		}
		mockValidator := &mockValidator{}
		importer := &Importer{
			Validator: mockValidator,
			GSBucket:  testGSBucket,
		}

		t.Run("happy path", func(t *ftt.Test) {
			t.Run("success import", func(t *ftt.Test) {
				mockGtClient.EXPECT().Log(gomock.Any(), protoutil.MatcherEqual(
					&gitilespb.LogRequest{
						Project:    "infradata/config",
						Committish: "refs/heads/main",
						Path:       "dev-configs/myservice",
						PageSize:   1,
					},
				)).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)
				tarGzContent, err := buildTarGz(map[string]any{"file1": "file1 content", "sub_dir/file2": "file2 content", "sub_dir/": "", "empty_file": ""})
				assert.Loosely(t, err, should.BeNil)
				mockGtClient.EXPECT().Archive(gomock.Any(), protoutil.MatcherEqual(
					&gitilespb.ArchiveRequest{
						Project: "infradata/config",
						Ref:     latestCommit.Id,
						Path:    "dev-configs/myservice",
						Format:  gitilespb.ArchiveRequest_GZIP,
					},
				)).Return(&gitilespb.ArchiveResponse{
					Contents: tarGzContent,
				}, nil)
				var recordedGSPaths []gs.Path
				mockGsClient.EXPECT().UploadIfMissing(
					gomock.Any(), gomock.Eq(testGSBucket),
					gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
					func(ctx context.Context, bucket, object string, data []byte, attrsModifyFn func(*storage.ObjectAttrs)) (bool, error) {
						recordedGSPaths = append(recordedGSPaths, gs.MakePath(bucket, object))
						return true, nil
					},
				)

				err = importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))
				assert.Loosely(t, err, should.BeNil)
				cfgSet := &model.ConfigSet{
					ID: config.MustServiceSet("myservice"),
				}
				attempt := &model.ImportAttempt{
					ConfigSet: datastore.KeyForObj(ctx, cfgSet),
				}
				revKey := datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice", model.RevisionKind, latestCommit.Id)
				var files []*model.File
				assert.Loosely(t, datastore.Get(ctx, cfgSet, attempt), should.BeNil)
				assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.FileKind).Ancestor(revKey), &files), should.BeNil)

				assert.Loosely(t, cfgSet.Location, should.Resemble(&cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  "refs/heads/main",
							Path: "dev-configs/myservice",
						},
					},
				}))
				assert.Loosely(t, cfgSet.LatestRevision.Location, should.Resemble(&cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice",
						},
					},
				}))
				// Drop the `Location` as model ConfigSet has to use ShouldResemble which
				// will not work if it contains proto. Same for other tests.
				cfgSet.Location = nil
				cfgSet.LatestRevision.Location = nil
				assert.Loosely(t, cfgSet, should.Resemble(&model.ConfigSet{
					ID:             "services/myservice",
					LatestRevision: expectedLatestRevInfo,
					Version:        model.CurrentCfgSetVersion,
				}))

				assert.Loosely(t, files, should.HaveLength(3))
				sort.Slice(files, func(i, j int) bool {
					return strings.Compare(files[i].Path, files[j].Path) < 0
				})
				assert.Loosely(t, files[0].Location, should.Resemble(&cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice/empty_file",
						},
					},
				}))
				files[0].Location = nil
				expectedSha256, expectedContent0, err := hashAndCompressConfig(bytes.NewBuffer([]byte("")))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, files[0], should.Resemble(&model.File{
					Path:          "empty_file",
					Revision:      revKey,
					CreateTime:    datastore.RoundTime(clock.Now(ctx).UTC()),
					Content:       expectedContent0,
					ContentSHA256: expectedSha256,
					GcsURI:        gs.MakePath(testGSBucket, fmt.Sprintf("%s/sha256/%s", common.GSProdCfgFolder, expectedSha256)),
				}))
				assert.Loosely(t, files[1].Location, should.Resemble(&cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice/file1",
						},
					},
				}))
				files[1].Location = nil
				expectedSha256, expectedContent1, err := hashAndCompressConfig(bytes.NewBuffer([]byte("file1 content")))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, files[1], should.Resemble(&model.File{
					Path:          "file1",
					Revision:      revKey,
					CreateTime:    datastore.RoundTime(clock.Now(ctx).UTC()),
					Content:       expectedContent1,
					ContentSHA256: expectedSha256,
					Size:          int64(len("file1 content")),
					GcsURI:        gs.MakePath(testGSBucket, fmt.Sprintf("%s/sha256/%s", common.GSProdCfgFolder, expectedSha256)),
				}))
				assert.Loosely(t, files[2].Location, should.Resemble(&cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice/sub_dir/file2",
						},
					},
				}))
				files[2].Location = nil
				expectedSha256, expectedContent2, err := hashAndCompressConfig(bytes.NewBuffer([]byte("file2 content")))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, files[2], should.Resemble(&model.File{
					Path:          "sub_dir/file2",
					Revision:      revKey,
					CreateTime:    datastore.RoundTime(clock.Now(ctx).UTC()),
					Content:       expectedContent2,
					ContentSHA256: expectedSha256,
					Size:          int64(len("file2 content")),
					GcsURI:        gs.MakePath(testGSBucket, fmt.Sprintf("%s/sha256/%s", common.GSProdCfgFolder, expectedSha256)),
				}))

				assert.Loosely(t, recordedGSPaths, should.HaveLength(len(files)))
				sort.Slice(recordedGSPaths, func(i, j int) bool {
					return strings.Compare(string(recordedGSPaths[i]), string(recordedGSPaths[j])) < 0
				})
				expectedGSPaths := make([]gs.Path, len(files))
				for i, f := range files {
					expectedGSPaths[i] = f.GcsURI
				}
				sort.Slice(expectedGSPaths, func(i, j int) bool {
					return strings.Compare(string(expectedGSPaths[i]), string(expectedGSPaths[j])) < 0
				})
				assert.Loosely(t, recordedGSPaths, should.Resemble(expectedGSPaths))

				assert.Loosely(t, attempt.Revision.Location, should.Resemble(&cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice",
						},
					},
				}))
				attempt.Revision.Location = nil
				assert.Loosely(t, attempt, should.Resemble(&model.ImportAttempt{
					ConfigSet: datastore.KeyForObj(ctx, cfgSet),
					Revision:  expectedLatestRevInfo,
					Success:   true,
					Message:   "Imported",
				}))
			})

			t.Run("same git revision", func(t *ftt.Test) {
				loc := &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  "refs/heads/main",
							Path: "dev-configs/myservice",
						},
					},
				}
				cfgSetBeforeImport := &model.ConfigSet{
					ID:             config.MustServiceSet("myservice"),
					Location:       loc,
					LatestRevision: model.RevisionInfo{ID: latestCommit.Id},
				}

				assert.Loosely(t, datastore.Put(ctx, cfgSetBeforeImport), should.BeNil)

				expectCall := func() {
					mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
						Log: []*git.Commit{latestCommit},
					}, nil)
				}

				t.Run("last attempt succeeded", func(t *ftt.Test) {
					expectCall()
					lastAttempt := &model.ImportAttempt{
						ConfigSet: datastore.KeyForObj(ctx, cfgSetBeforeImport),
						Success:   true,
						Message:   "imported",
					}
					assert.Loosely(t, datastore.Put(ctx, cfgSetBeforeImport, lastAttempt), should.BeNil)

					err := importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))

					assert.Loosely(t, err, should.BeNil)
					cfgSetAfterImport := &model.ConfigSet{
						ID: config.MustServiceSet("myservice"),
					}
					assert.Loosely(t, datastore.Get(ctx, cfgSetAfterImport), should.BeNil)
					assert.Loosely(t, cfgSetAfterImport.Location, should.Resemble(loc))
					cfgSetAfterImport.Location = nil
					cfgSetBeforeImport.Location = nil
					assert.Loosely(t, cfgSetAfterImport, should.Resemble(cfgSetBeforeImport))
					attempt := &model.ImportAttempt{ConfigSet: datastore.KeyForObj(ctx, cfgSetAfterImport)}
					assert.Loosely(t, datastore.Get(ctx, attempt), should.BeNil)
					assert.Loosely(t, attempt.Revision.Location, should.Resemble(&cfgcommonpb.Location{
						Location: &cfgcommonpb.Location_GitilesLocation{
							GitilesLocation: &cfgcommonpb.GitilesLocation{
								Repo: "https://a.googlesource.com/infradata/config",
								Ref:  latestCommit.Id,
								Path: "dev-configs/myservice",
							},
						},
					}))
					attempt.Revision.Location = nil
					assert.Loosely(t, attempt, should.Resemble(&model.ImportAttempt{
						ConfigSet: datastore.KeyForObj(ctx, cfgSetAfterImport),
						Success:   true,
						Message:   "Up-to-date",
						Revision: model.RevisionInfo{
							ID:             latestCommit.Id,
							CommitTime:     latestCommit.Committer.Time.AsTime(),
							CommitterEmail: latestCommit.Committer.Email,
							AuthorEmail:    latestCommit.Author.Email,
						},
					}))
				})

				t.Run("last attempt succeeded but with validation msg", func(t *ftt.Test) {
					expectCall()
					lastAttempt := &model.ImportAttempt{
						ConfigSet: datastore.KeyForObj(ctx, cfgSetBeforeImport),
						Success:   true,
						Message:   "imported with warnings",
						ValidationResult: &cfgcommonpb.ValidationResult{
							Messages: []*cfgcommonpb.ValidationResult_Message{
								{
									Path:     "foo.cfg",
									Severity: cfgcommonpb.ValidationResult_WARNING,
									Text:     "there is a warning",
								},
							},
						},
					}
					assert.Loosely(t, datastore.Put(ctx, lastAttempt), should.BeNil)

					tarGzContent, err := buildTarGz(map[string]any{"foo.cfg": "content"})
					assert.Loosely(t, err, should.BeNil)
					mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{
						Contents: tarGzContent,
					}, nil)
					mockGsClient.EXPECT().UploadIfMissing(
						gomock.Any(), gomock.Eq(testGSBucket),
						gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
					mockValidator.result = lastAttempt.ValidationResult

					err = importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))

					assert.Loosely(t, err, should.BeNil)
					currentAttempt := &model.ImportAttempt{ConfigSet: datastore.KeyForObj(ctx, cfgSetBeforeImport)}
					assert.Loosely(t, datastore.Get(ctx, currentAttempt), should.BeNil)
					assert.Loosely(t, currentAttempt.ValidationResult, should.Resemble(lastAttempt.ValidationResult))
					assert.Loosely(t, currentAttempt.Success, should.BeTrue)
				})

				t.Run("last attempt not succeeded", func(t *ftt.Test) {
					expectCall()
					lastAttempt := &model.ImportAttempt{
						ConfigSet: datastore.KeyForObj(ctx, cfgSetBeforeImport),
						Success:   false,
						Message:   "transient gitilies error",
					}
					assert.Loosely(t, datastore.Put(ctx, lastAttempt), should.BeNil)

					tarGzContent, err := buildTarGz(map[string]any{"foo.cfg": "content"})
					assert.Loosely(t, err, should.BeNil)
					mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{
						Contents: tarGzContent,
					}, nil)
					mockGsClient.EXPECT().UploadIfMissing(
						gomock.Any(), gomock.Eq(testGSBucket),
						gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)

					err = importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))

					assert.Loosely(t, err, should.BeNil)
					currentAttempt := &model.ImportAttempt{ConfigSet: datastore.KeyForObj(ctx, cfgSetBeforeImport)}
					assert.Loosely(t, datastore.Get(ctx, currentAttempt), should.BeNil)
					assert.Loosely(t, currentAttempt.Success, should.BeTrue)
				})
			})

			t.Run("empty archive", func(t *ftt.Test) {
				mockGtClient.EXPECT().Log(gomock.Any(), protoutil.MatcherEqual(
					&gitilespb.LogRequest{
						Project:    "infradata/config",
						Committish: "refs/heads/main",
						Path:       "dev-configs/myservice",
						PageSize:   1,
					},
				)).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{}, nil)

				err := importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))

				assert.Loosely(t, err, should.BeNil)
				cfgSet := &model.ConfigSet{
					ID: config.MustServiceSet("myservice"),
				}
				attempt := &model.ImportAttempt{
					ConfigSet: datastore.KeyForObj(ctx, cfgSet),
				}
				revKey := datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice", model.RevisionKind, latestCommit.Id)
				assert.Loosely(t, datastore.Get(ctx, cfgSet, attempt), should.BeNil)
				var files []*model.File
				assert.Loosely(t, datastore.Run(ctx, datastore.NewQuery(model.FileKind).Ancestor(revKey), func(f *model.File) {
					files = append(files, f)
				}), should.BeNil)
				assert.Loosely(t, files, should.HaveLength(0))

				assert.Loosely(t, cfgSet.Location, should.Resemble(&cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  "refs/heads/main",
							Path: "dev-configs/myservice",
						},
					},
				}))
				assert.Loosely(t, cfgSet.LatestRevision.Location, should.Resemble(&cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice",
						},
					},
				}))

				cfgSet.Location = nil
				cfgSet.LatestRevision.Location = nil
				assert.Loosely(t, cfgSet, should.Resemble(&model.ConfigSet{
					ID:             "services/myservice",
					LatestRevision: expectedLatestRevInfo,
					Version:        model.CurrentCfgSetVersion,
				}))

				assert.Loosely(t, attempt.Success, should.BeTrue)
				assert.Loosely(t, attempt.Message, should.Equal("No Configs. Imported as empty"))
			})

			t.Run("config set location change", func(t *ftt.Test) {
				cfgSetBeforeImport := &model.ConfigSet{
					ID: config.MustServiceSet("myservice"),
					Location: &cfgcommonpb.Location{
						Location: &cfgcommonpb.Location_GitilesLocation{
							GitilesLocation: &cfgcommonpb.GitilesLocation{
								Repo: "https://a.googlesource.com/infradata/config",
								Ref:  "stale",
								Path: "dev-configs/myservice",
							},
						},
					},
					LatestRevision: model.RevisionInfo{ID: latestCommit.Id},
				}
				assert.Loosely(t, datastore.Put(ctx, cfgSetBeforeImport), should.BeNil)
				mockGtClient.EXPECT().Log(gomock.Any(), protoutil.MatcherEqual(
					&gitilespb.LogRequest{
						Project:    "infradata/config",
						Committish: "refs/heads/main",
						Path:       "dev-configs/myservice",
						PageSize:   1,
					},
				)).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{}, nil)

				err := importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))

				assert.Loosely(t, err, should.BeNil)
				cfgSetAfterImport := &model.ConfigSet{
					ID: config.MustServiceSet("myservice"),
				}
				assert.Loosely(t, datastore.Get(ctx, cfgSetAfterImport), should.BeNil)
				assert.Loosely(t, cfgSetAfterImport.Location.GetGitilesLocation().Ref, should.NotEqual(cfgSetBeforeImport.Location.GetGitilesLocation().Ref))
				assert.Loosely(t, cfgSetAfterImport.Location, should.Resemble(&cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  "refs/heads/main",
							Path: "dev-configs/myservice",
						},
					},
				}))
			})

			t.Run("no logs", func(t *ftt.Test) {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{},
				}, nil)
				err := importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))
				assert.Loosely(t, err, should.BeNil)
				attempt := &model.ImportAttempt{
					ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice"),
				}
				assert.Loosely(t, datastore.Get(ctx, attempt), should.BeNil)
				assert.Loosely(t, attempt.Success, should.BeTrue)
				assert.Loosely(t, attempt.Message, should.ContainSubstring("no commit logs"))
			})

			t.Run("validate", func(t *ftt.Test) {
				expect := func() {
					mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
						Log: []*git.Commit{latestCommit},
					}, nil)

					tarGzContent, err := buildTarGz(map[string]any{"foo.cfg": "content"})
					assert.Loosely(t, err, should.BeNil)
					mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{
						Contents: tarGzContent,
					}, nil)
					mockGsClient.EXPECT().UploadIfMissing(
						gomock.Any(), gomock.Eq(testGSBucket),
						gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
				}

				t.Run("has warning", func(t *ftt.Test) {
					expect()
					mockValidator.result = &cfgcommonpb.ValidationResult{
						Messages: []*cfgcommonpb.ValidationResult_Message{
							{
								Path:     "foo.cfg",
								Severity: cfgcommonpb.ValidationResult_WARNING,
								Text:     "this is a warning",
							},
						},
					}
					err := importer.ImportConfigSet(ctx, cs)
					assert.Loosely(t, err, should.BeNil)
					revKey := datastore.MakeKey(ctx, model.ConfigSetKind, string(cs), model.RevisionKind, latestCommit.Id)
					var files []*model.File
					assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.FileKind).Ancestor(revKey), &files), should.BeNil)
					assert.Loosely(t, files, should.HaveLength(1))
					assert.Loosely(t, files[0].Path, should.Equal("foo.cfg"))
					attempt := &model.ImportAttempt{
						ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, string(cs)),
					}
					assert.Loosely(t, datastore.Get(ctx, attempt), should.BeNil)
					assert.Loosely(t, attempt.Success, should.BeTrue)
					assert.Loosely(t, attempt.Revision.ID, should.Equal(latestCommit.Id))
					assert.Loosely(t, attempt.Message, should.Equal("Imported with warnings"))
					assert.Loosely(t, attempt.ValidationResult, should.Resemble(mockValidator.result))
				})

				t.Run("has error", func(t *ftt.Test) {
					mockValidator.result = &cfgcommonpb.ValidationResult{
						Messages: []*cfgcommonpb.ValidationResult_Message{
							{
								Path:     "foo.cfg",
								Severity: cfgcommonpb.ValidationResult_ERROR,
								Text:     "this is an error",
							},
						},
					}

					check := func(t testing.TB) {
						t.Helper()
						revKey := datastore.MakeKey(ctx, model.ConfigSetKind, string(cs), model.RevisionKind, latestCommit.Id)
						var files []*model.File
						assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.FileKind).Ancestor(revKey), &files), should.BeNil, truth.LineContext())
						assert.Loosely(t, files, should.BeEmpty, truth.LineContext())
						attempt := &model.ImportAttempt{
							ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, string(cs)),
						}
						assert.Loosely(t, datastore.Get(ctx, attempt), should.BeNil, truth.LineContext())
						assert.Loosely(t, attempt.Success, should.BeFalse, truth.LineContext())
						assert.Loosely(t, attempt.Message, should.Equal("Invalid config"), truth.LineContext())
						assert.Loosely(t, attempt.Revision.ID, should.Equal(latestCommit.Id), truth.LineContext())
						assert.Loosely(t, attempt.ValidationResult, should.Resemble(mockValidator.result), truth.LineContext())
					}

					t.Run("author email is google", func(t *ftt.Test) {
						expect()
						latestCommit.Author = &git.Commit_User{
							Name:  "author",
							Email: "author@google.com",
						}

						err := importer.ImportConfigSet(ctx, cs)
						assert.Loosely(t, err, should.BeNil)

						assert.Loosely(t, metrics.RejectedCfgImportCounter.Get(ctx, string(cs), latestCommit.Id, "author"), should.Equal(1))
						check(t)
					})

					t.Run("author email is non-google", func(t *ftt.Test) {
						expect()
						latestCommit.Author = &git.Commit_User{
							Name:  "author",
							Email: "author@chrmoium.org",
						}

						err := importer.ImportConfigSet(ctx, cs)
						assert.Loosely(t, err, should.BeNil)

						assert.Loosely(t, metrics.RejectedCfgImportCounter.Get(ctx, string(cs), latestCommit.Id, ""), should.Equal(1))
						check(t)
					})
				})
			})

			t.Run("large config", func(t *ftt.Test) {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)
				// Construct incompressible data which is larger than compressedContentLimit.
				incompressible := make([]byte, compressedContentLimit+1024*1024)
				_, err := rand.New(rand.NewSource(1234)).Read(incompressible)
				assert.Loosely(t, err, should.BeNil)
				compressed, err := gzipCompress(incompressible)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(compressed) > compressedContentLimit, should.BeTrue)

				tarGzContent, err := buildTarGz(map[string]any{"large": incompressible})
				assert.Loosely(t, err, should.BeNil)
				expectedSha256 := sha256.Sum256(incompressible)
				expectedGsFileName := "configs/sha256/" + hex.EncodeToString(expectedSha256[:])
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{
					Contents: tarGzContent,
				}, nil)
				mockGsClient.EXPECT().UploadIfMissing(
					gomock.Any(), gomock.Eq(testGSBucket),
					gomock.Eq(expectedGsFileName), gomock.Any(), gomock.Any()).Return(true, nil)

				err = importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))

				assert.Loosely(t, err, should.BeNil)
				revKey := datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice", model.RevisionKind, latestCommit.Id)
				var files []*model.File
				assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.FileKind).Ancestor(revKey), &files), should.BeNil)
				assert.Loosely(t, files, should.HaveLength(1))
				file := files[0]
				assert.Loosely(t, file.Path, should.Equal("large"))
				assert.Loosely(t, file.Content, should.BeNil)
				assert.Loosely(t, file.GcsURI, should.Equal(gs.MakePath(testGSBucket, expectedGsFileName)))
			})
		})

		t.Run("unhappy path", func(t *ftt.Test) {
			t.Run("bad config set format", func(t *ftt.Test) {
				err := importer.ImportConfigSet(ctx, config.Set("bad"))
				assert.Loosely(t, err, should.ErrLike("Invalid config set"))
			})

			t.Run("project doesn't exist ", func(t *ftt.Test) {
				testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
					common.ProjRegistryFilePath: &cfgcommonpb.ProjectsCfg{
						Projects: []*cfgcommonpb.Project{
							{
								Id: "proj",
								Location: &cfgcommonpb.Project_GitilesLocation{
									GitilesLocation: &cfgcommonpb.GitilesLocation{
										Repo: "https://chromium.googlesource.com/infra/infra",
										Ref:  "refs/heads/main",
										Path: "generated",
									},
								},
							},
						},
					},
				})
				err := importer.ImportConfigSet(ctx, config.MustProjectSet("unknown_proj"))
				assert.Loosely(t, err, should.ErrLike(`project "unknown_proj" not exist or has no gitiles location`))
				assert.Loosely(t, ErrFatalTag.In(err), should.BeTrue)
			})

			t.Run("no project gitiles location", func(t *ftt.Test) {
				testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
					common.ProjRegistryFilePath: &cfgcommonpb.ProjectsCfg{
						Projects: []*cfgcommonpb.Project{
							{Id: "proj"},
						},
					},
				})
				err := importer.ImportConfigSet(ctx, config.MustProjectSet("proj"))
				assert.Loosely(t, err, should.ErrLike(`project "proj" not exist or has no gitiles location`))
				assert.Loosely(t, ErrFatalTag.In(err), should.BeTrue)
			})

			t.Run("cannot fetch logs", func(t *ftt.Test) {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(nil, errors.New("gitiles internal errors"))
				err := importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))
				assert.Loosely(t, err, should.ErrLike("cannot fetch logs"))
				assert.Loosely(t, err, should.ErrLike("gitiles internal errors"))
				attempt := &model.ImportAttempt{
					ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice"),
				}
				assert.Loosely(t, datastore.Get(ctx, attempt), should.BeNil)
				assert.Loosely(t, attempt.Success, should.BeFalse)
				assert.Loosely(t, attempt.Message, should.ContainSubstring("cannot fetch logs"))
			})

			t.Run("bad tar file", func(t *ftt.Test) {
				loc := &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  "refs/heads/main",
							Path: "dev-configs/myservice",
						},
					},
				}
				cfgSetBeforeImport := &model.ConfigSet{
					ID:             config.MustServiceSet("myservice"),
					Location:       loc,
					LatestRevision: model.RevisionInfo{ID: "old revision"},
				}
				assert.Loosely(t, datastore.Put(ctx, cfgSetBeforeImport), should.BeNil)
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{
					Contents: []byte("invalid .tar.gz content"),
				}, nil)

				err := importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))

				assert.Loosely(t, err, should.ErrLike("Failed to import services/myservice revision latest revision"))
				cfgSetAfterImport := &model.ConfigSet{
					ID: config.MustServiceSet("myservice"),
				}
				attempt := &model.ImportAttempt{
					ConfigSet: datastore.KeyForObj(ctx, cfgSetAfterImport),
				}
				assert.Loosely(t, datastore.Get(ctx, cfgSetAfterImport, attempt), should.BeNil)

				assert.Loosely(t, cfgSetAfterImport.Location, should.Resemble(cfgSetBeforeImport.Location))
				assert.Loosely(t, cfgSetAfterImport.LatestRevision.ID, should.Equal(cfgSetBeforeImport.LatestRevision.ID))

				assert.Loosely(t, attempt.Success, should.BeFalse)
				assert.Loosely(t, attempt.Message, should.ContainSubstring("Failed to import services/myservice revision latest revision"))
			})

			t.Run("failed to upload to GCS", func(t *ftt.Test) {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)
				// Construct incompressible data which is larger than compressedContentLimit.
				incompressible := make([]byte, compressedContentLimit+1024*1024)
				_, err := rand.New(rand.NewSource(1234)).Read(incompressible)
				assert.Loosely(t, err, should.BeNil)
				compressed, err := gzipCompress(incompressible)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(compressed) > compressedContentLimit, should.BeTrue)
				tarGzContent, err := buildTarGz(map[string]any{"large": incompressible})
				assert.Loosely(t, err, should.BeNil)
				expectedSha256 := sha256.Sum256(incompressible)
				expectedGsFileName := "configs/sha256/" + hex.EncodeToString(expectedSha256[:])
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{
					Contents: tarGzContent,
				}, nil)
				mockGsClient.EXPECT().UploadIfMissing(
					gomock.Any(), gomock.Eq(testGSBucket),
					gomock.Eq(expectedGsFileName), gomock.Any(), gomock.Any()).Return(false, errors.New("GCS internal error"))

				err = importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))

				assert.Loosely(t, err, should.ErrLike("failed to upload file"))
				assert.Loosely(t, err, should.ErrLike("GCS internal error"))
				revKey := datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice", model.RevisionKind, latestCommit.Id)
				var files []*model.File
				assert.Loosely(t, datastore.GetAll(ctx, datastore.NewQuery(model.FileKind).Ancestor(revKey), &files), should.BeNil)
				assert.Loosely(t, files, should.HaveLength(0))

				attempt := &model.ImportAttempt{
					ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice"),
				}
				assert.Loosely(t, datastore.Get(ctx, attempt), should.BeNil)
				assert.Loosely(t, attempt.Success, should.BeFalse)
				assert.Loosely(t, attempt.Message, should.ContainSubstring("failed to upload file"))
			})

			t.Run("validation error", func(t *ftt.Test) {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)

				tarGzContent, err := buildTarGz(map[string]any{"foo.cfg": "content"})
				assert.Loosely(t, err, should.BeNil)
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{
					Contents: tarGzContent,
				}, nil)
				mockGsClient.EXPECT().UploadIfMissing(
					gomock.Any(), gomock.Eq(testGSBucket),
					gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)

				mockValidator.err = errors.New("something went wrong during validation")
				err = importer.ImportConfigSet(ctx, cs)
				assert.Loosely(t, err, should.ErrLike("something went wrong during validation"))
			})
		})
	})
}

func TestReImport(t *testing.T) {
	t.Parallel()

	ftt.Run("Reimport", t, func(t *ftt.Test) {
		rsp := httptest.NewRecorder()
		rctx := &router.Context{
			Writer: rsp,
		}

		mockValidator := &mockValidator{}
		importer := &Importer{
			Validator: mockValidator,
		}
		ctx := testutil.SetupContext()
		userID := identity.Identity("user:user@example.com")
		fakeAuthDB := authtest.NewFakeDB()
		testutil.InjectSelfConfigs(ctx, t, map[string]proto.Message{
			common.ACLRegistryFilePath: &cfgcommonpb.AclCfg{
				ServiceAccessGroup:   "service-access-group",
				ServiceReimportGroup: "service-reimport-group",
			},
		})
		fakeAuthDB.AddMocks(
			authtest.MockMembership(userID, "service-access-group"),
			authtest.MockMembership(userID, "service-reimport-group"),
		)
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: userID,
			FakeDB:   fakeAuthDB,
		})

		t.Run("no ConfigSet param", func(t *ftt.Test) {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			importer.Reimport(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring("config set is not specified"))
		})

		t.Run("invalid config set", func(t *ftt.Test) {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "ConfigSet", Value: "badCfgSet"},
			}
			importer.Reimport(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusBadRequest))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring(`invalid config set: unknown domain "badCfgSet" for config set "badCfgSet"`))
		})

		t.Run("no permission", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:random@example.com"),
				FakeDB:   authtest.NewFakeDB(),
			})
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "ConfigSet", Value: "services/myservice"},
			}
			importer.Reimport(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusForbidden))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring(`"user:random@example.com" is not allowed to reimport services/myservice`))
		})

		t.Run("config set not found", func(t *ftt.Test) {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "ConfigSet", Value: "services/myservice"},
			}
			importer.Reimport(rctx)
			assert.Loosely(t, rsp.Code, should.Equal(http.StatusNotFound))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring(`"services/myservice" is not found`))
		})

		t.Run("ok", func(t *ftt.Test) {
			ctx = settings.WithGlobalConfigLoc(ctx, &cfgcommonpb.GitilesLocation{
				Repo: "https://a.googlesource.com/infradata/config",
				Ref:  "refs/heads/main",
				Path: "dev-configs",
			})
			testutil.InjectConfigSet(ctx, t, "services/myservice", nil)
			ctl := gomock.NewController(t)
			mockGtClient := mock_gitiles.NewMockGitilesClient(ctl)
			ctx = context.WithValue(ctx, &clients.MockGitilesClientKey, mockGtClient)
			mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{}, nil)

			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "ConfigSet", Value: "services/myservice"},
			}
			importer.Reimport(rctx)

			assert.Loosely(t, rsp.Code, should.Equal(http.StatusOK))
		})

		t.Run("internal error", func(t *ftt.Test) {
			ctx = settings.WithGlobalConfigLoc(ctx, &cfgcommonpb.GitilesLocation{
				Repo: "https://a.googlesource.com/infradata/config",
				Ref:  "refs/heads/main",
				Path: "dev-configs",
			})
			testutil.InjectConfigSet(ctx, t, "services/myservice", nil)
			ctl := gomock.NewController(t)
			mockGtClient := mock_gitiles.NewMockGitilesClient(ctl)
			ctx = context.WithValue(ctx, &clients.MockGitilesClientKey, mockGtClient)
			mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(nil, errors.New("gitiles internal error"))

			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "ConfigSet", Value: "services/myservice"},
			}
			importer.Reimport(rctx)

			assert.Loosely(t, rsp.Code, should.Equal(http.StatusInternalServerError))
			assert.Loosely(t, rsp.Body.String(), should.ContainSubstring(`error when reimporting "services/myservice"`))
		})
	})
}

func buildTarGz(raw map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, body := range raw {
		hdr := &tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
		}
		if strings.HasSuffix(name, "/") {
			hdr.Typeflag = tar.TypeDir
		}

		var bodyBytes []byte
		switch v := body.(type) {
		case string:
			bodyBytes = []byte(body.(string))
		case []byte:
			bodyBytes = body.([]byte)
		default:
			return nil, errors.Fmt("unsupported type %T for %s", v, name)
		}
		hdr.Size = int64(len(bodyBytes))
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(bodyBytes); err != nil {
			return nil, err
		}
	}
	if err := tw.Flush(); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gzw.Flush(); err != nil {
		return nil, err
	}
	if err := gzw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func getCfgSetsInTaskQueue(sch *tqtesting.Scheduler) []string {
	cfgSets := make([]string, len(sch.Tasks()))
	for i, task := range sch.Tasks() {
		cfgSets[i] = task.Payload.(*taskpb.ImportConfigs).ConfigSet
	}
	sort.Slice(cfgSets, func(i, j int) bool { return cfgSets[i] < cfgSets[j] })
	return cfgSets
}

func gzipCompress(b []byte) ([]byte, error) {
	buf := &bytes.Buffer{}
	gw := gzip.NewWriter(buf)
	if _, err := gw.Write(b); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
