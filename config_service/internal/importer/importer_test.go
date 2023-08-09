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
	"compress/gzip"
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
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/internal/settings"
	"go.chromium.org/luci/config_service/internal/taskpb"
	"go.chromium.org/luci/config_service/internal/validation"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestImportAllConfigs(t *testing.T) {
	t.Parallel()

	Convey("import all configs", t, func() {
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

		Convey("ok", func() {
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

			Convey("projects.cfg File entity not exist", func() {
				err := importAllConfigs(ctx, disp)
				So(err, ShouldBeNil)
				cfgSetsInQueue := getCfgSetsInTaskQueue(sch)
				So(cfgSetsInQueue, ShouldResemble, []string{"services/service1", "services/service2"})
			})

			Convey("projects.cfg File entity exist", func() {
				testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
					"projects.cfg": &cfgcommonpb.ProjectsCfg{
						Projects: []*cfgcommonpb.Project{
							{Id: "proj1"},
						},
					},
				})
				err := importAllConfigs(ctx, disp)
				So(err, ShouldBeNil)
				cfgSetsInQueue := getCfgSetsInTaskQueue(sch)
				So(cfgSetsInQueue, ShouldResemble, []string{"projects/proj1", "services/service1", "services/service2"})
			})

			Convey("delete stale config set", func() {
				stale := &model.ConfigSet{ID: config.MustServiceSet("stale")}
				So(datastore.Put(ctx, stale), ShouldBeNil)
				err := importAllConfigs(ctx, disp)
				So(err, ShouldBeNil)
				So(datastore.Get(ctx, stale), ShouldEqual, datastore.ErrNoSuchEntity)
			})
		})

		Convey("error", func() {
			Convey("gitiles", func() {
				mockClient.EXPECT().ListFiles(gomock.Any(), gomock.Any()).Return(nil, errors.New("gitiles error"))
				err := importAllConfigs(ctx, disp)
				So(err, ShouldErrLike, "failed to load service config sets: failed to call Gitiles to list files: gitiles error")
			})

			Convey("bad projects.cfg content", func() {
				mockClient.EXPECT().ListFiles(gomock.Any(), gomock.Any()).Return(&gitilespb.ListFilesResponse{}, nil)
				testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
					"projects.cfg": &cfgcommonpb.ServicesCfg{
						Services: []*cfgcommonpb.Service{
							{Id: "my-service"},
						}}, // bad type
				})
				So(importAllConfigs(ctx, disp), ShouldErrLike, `failed to load project config sets: failed to unmarshal file "projects.cfg": proto`)
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

	Convey("import single ConfigSet", t, func() {
		ctx := testutil.SetupContext()
		ctx = settings.WithGlobalConfigLoc(ctx, &cfgcommonpb.GitilesLocation{
			Repo: "https://a.googlesource.com/infradata/config",
			Ref:  "refs/heads/main",
			Path: "dev-configs",
		})
		ctl := gomock.NewController(t)
		defer ctl.Finish()
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
		}
		expectedLatestRevInfo := model.RevisionInfo{
			ID:             latestCommit.Id,
			CommitTime:     latestCommit.Committer.Time.AsTime(),
			CommitterEmail: latestCommit.Committer.Email,
		}
		mockValidator := &mockValidator{}
		importer := &Importer{
			Validator: mockValidator,
			GSBucket:  testGSBucket,
		}

		Convey("happy path", func() {
			Convey("success import", func() {
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
				So(err, ShouldBeNil)
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
				So(err, ShouldBeNil)
				cfgSet := &model.ConfigSet{
					ID: config.MustServiceSet("myservice"),
				}
				attempt := &model.ImportAttempt{
					ConfigSet: datastore.KeyForObj(ctx, cfgSet),
				}
				revKey := datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice", model.RevisionKind, latestCommit.Id)
				var files []*model.File
				So(datastore.Get(ctx, cfgSet, attempt), ShouldBeNil)
				So(datastore.GetAll(ctx, datastore.NewQuery(model.FileKind).Ancestor(revKey), &files), ShouldBeNil)

				So(cfgSet.Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  "refs/heads/main",
							Path: "dev-configs/myservice",
						},
					},
				})
				So(cfgSet.LatestRevision.Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice",
						},
					},
				})
				// Drop the `Location` as model ConfigSet has to use ShouldResemble which
				// will not work if it contains proto. Same for other tests.
				cfgSet.Location = nil
				cfgSet.LatestRevision.Location = nil
				So(cfgSet, ShouldResemble, &model.ConfigSet{
					ID:             "services/myservice",
					LatestRevision: expectedLatestRevInfo,
				})

				So(files, ShouldHaveLength, 3)
				sort.Slice(files, func(i, j int) bool {
					return strings.Compare(files[i].Path, files[j].Path) < 0
				})
				So(files[0].Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice/empty_file",
						},
					},
				})
				files[0].Location = nil
				expectedSha256, expectedContent0, err := hashAndCompressConfig(bytes.NewBuffer([]byte("")))
				So(err, ShouldBeNil)
				So(files[0], ShouldResemble, &model.File{
					Path:          "empty_file",
					Revision:      revKey,
					CreateTime:    datastore.RoundTime(clock.Now(ctx).UTC()),
					Content:       expectedContent0,
					ContentSHA256: expectedSha256,
					GcsURI:        gs.MakePath(testGSBucket, fmt.Sprintf("%s/sha256/%s", common.GSProdCfgFolder, expectedSha256)),
				})
				So(files[1].Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice/file1",
						},
					},
				})
				files[1].Location = nil
				expectedSha256, expectedContent1, err := hashAndCompressConfig(bytes.NewBuffer([]byte("file1 content")))
				So(err, ShouldBeNil)
				So(files[1], ShouldResemble, &model.File{
					Path:          "file1",
					Revision:      revKey,
					CreateTime:    datastore.RoundTime(clock.Now(ctx).UTC()),
					Content:       expectedContent1,
					ContentSHA256: expectedSha256,
					GcsURI:        gs.MakePath(testGSBucket, fmt.Sprintf("%s/sha256/%s", common.GSProdCfgFolder, expectedSha256)),
				})
				So(files[2].Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice/sub_dir/file2",
						},
					},
				})
				files[2].Location = nil
				expectedSha256, expectedContent2, err := hashAndCompressConfig(bytes.NewBuffer([]byte("file2 content")))
				So(err, ShouldBeNil)
				So(files[2], ShouldResemble, &model.File{
					Path:          "sub_dir/file2",
					Revision:      revKey,
					CreateTime:    datastore.RoundTime(clock.Now(ctx).UTC()),
					Content:       expectedContent2,
					ContentSHA256: expectedSha256,
					GcsURI:        gs.MakePath(testGSBucket, fmt.Sprintf("%s/sha256/%s", common.GSProdCfgFolder, expectedSha256)),
				})

				So(recordedGSPaths, ShouldHaveLength, len(files))
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
				So(recordedGSPaths, ShouldResemble, expectedGSPaths)

				So(attempt.Revision.Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice",
						},
					},
				})
				attempt.Revision.Location = nil
				So(attempt, ShouldResemble, &model.ImportAttempt{
					ConfigSet: datastore.KeyForObj(ctx, cfgSet),
					Revision:  expectedLatestRevInfo,
					Success:   true,
					Message:   "Imported",
				})
			})

			Convey("same git revision", func() {
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
				So(datastore.Put(ctx, cfgSetBeforeImport), ShouldBeNil)
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)

				err := importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))

				So(err, ShouldBeNil)
				cfgSetAfterImport := &model.ConfigSet{
					ID: config.MustServiceSet("myservice"),
				}
				So(datastore.Get(ctx, cfgSetAfterImport), ShouldBeNil)
				So(cfgSetAfterImport.Location, ShouldResembleProto, loc)
				cfgSetAfterImport.Location = nil
				cfgSetBeforeImport.Location = nil
				So(cfgSetAfterImport, ShouldResemble, cfgSetBeforeImport)
				attempt := &model.ImportAttempt{ConfigSet: datastore.KeyForObj(ctx, cfgSetAfterImport)}
				So(datastore.Get(ctx, attempt), ShouldBeNil)
				So(attempt.Success, ShouldBeTrue)
				So(attempt.Message, ShouldEqual, "Up-to-date")
			})

			Convey("empty archive", func() {
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

				So(err, ShouldBeNil)
				cfgSet := &model.ConfigSet{
					ID: config.MustServiceSet("myservice"),
				}
				attempt := &model.ImportAttempt{
					ConfigSet: datastore.KeyForObj(ctx, cfgSet),
				}
				revKey := datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice", model.RevisionKind, latestCommit.Id)
				So(datastore.Get(ctx, cfgSet, attempt), ShouldBeNil)
				var files []*model.File
				So(datastore.Run(ctx, datastore.NewQuery(model.FileKind).Ancestor(revKey), func(f *model.File) {
					files = append(files, f)
				}), ShouldBeNil)
				So(files, ShouldHaveLength, 0)

				So(cfgSet.Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  "refs/heads/main",
							Path: "dev-configs/myservice",
						},
					},
				})
				So(cfgSet.LatestRevision.Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice",
						},
					},
				})

				cfgSet.Location = nil
				cfgSet.LatestRevision.Location = nil
				So(cfgSet, ShouldResemble, &model.ConfigSet{
					ID:             "services/myservice",
					LatestRevision: expectedLatestRevInfo,
				})

				So(attempt.Success, ShouldBeTrue)
				So(attempt.Message, ShouldEqual, "No Configs. Imported as empty")
			})

			Convey("config set location change", func() {
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
				So(datastore.Put(ctx, cfgSetBeforeImport), ShouldBeNil)
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

				So(err, ShouldBeNil)
				cfgSetAfterImport := &model.ConfigSet{
					ID: config.MustServiceSet("myservice"),
				}
				So(datastore.Get(ctx, cfgSetAfterImport), ShouldBeNil)
				So(cfgSetAfterImport.Location.GetGitilesLocation().Ref, ShouldNotEqual, cfgSetBeforeImport.Location.GetGitilesLocation().Ref)
				So(cfgSetAfterImport.Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://a.googlesource.com/infradata/config",
							Ref:  "refs/heads/main",
							Path: "dev-configs/myservice",
						},
					},
				})
			})

			Convey("no logs", func() {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{},
				}, nil)
				err := importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))
				So(err, ShouldBeNil)
				attempt := &model.ImportAttempt{
					ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice"),
				}
				So(datastore.Get(ctx, attempt), ShouldBeNil)
				So(attempt.Success, ShouldBeTrue)
				So(attempt.Message, ShouldContainSubstring, "no commit logs")
			})

			Convey("validate", func() {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)

				tarGzContent, err := buildTarGz(map[string]any{"foo.cfg": "content"})
				So(err, ShouldBeNil)
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{
					Contents: tarGzContent,
				}, nil)
				mockGsClient.EXPECT().UploadIfMissing(
					gomock.Any(), gomock.Eq(testGSBucket),
					gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)

				Convey("has warning", func() {
					mockValidator.result = &cfgcommonpb.ValidationResult{
						Messages: []*cfgcommonpb.ValidationResult_Message{
							{
								Path:     "foo.cfg",
								Severity: cfgcommonpb.ValidationResult_WARNING,
								Text:     "this is a warning",
							},
						},
					}
					err = importer.ImportConfigSet(ctx, cs)
					So(err, ShouldBeNil)
					revKey := datastore.MakeKey(ctx, model.ConfigSetKind, string(cs), model.RevisionKind, latestCommit.Id)
					var files []*model.File
					So(datastore.GetAll(ctx, datastore.NewQuery(model.FileKind).Ancestor(revKey), &files), ShouldBeNil)
					So(files, ShouldHaveLength, 1)
					So(files[0].Path, ShouldEqual, "foo.cfg")
					attempt := &model.ImportAttempt{
						ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, string(cs)),
					}
					So(datastore.Get(ctx, attempt), ShouldBeNil)
					So(attempt.Success, ShouldBeTrue)
					So(attempt.Revision.ID, ShouldEqual, latestCommit.Id)
					So(attempt.Message, ShouldEqual, "Imported with warnings")
					So(attempt.ValidationResult, ShouldResembleProto, mockValidator.result)
				})
				Convey("has error", func() {
					mockValidator.result = &cfgcommonpb.ValidationResult{
						Messages: []*cfgcommonpb.ValidationResult_Message{
							{
								Path:     "foo.cfg",
								Severity: cfgcommonpb.ValidationResult_ERROR,
								Text:     "this is an error",
							},
						},
					}
					err = importer.ImportConfigSet(ctx, cs)
					So(err, ShouldBeNil)
					revKey := datastore.MakeKey(ctx, model.ConfigSetKind, string(cs), model.RevisionKind, latestCommit.Id)
					var files []*model.File
					So(datastore.GetAll(ctx, datastore.NewQuery(model.FileKind).Ancestor(revKey), &files), ShouldBeNil)
					So(files, ShouldBeEmpty)
					attempt := &model.ImportAttempt{
						ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, string(cs)),
					}
					So(datastore.Get(ctx, attempt), ShouldBeNil)
					So(attempt.Success, ShouldBeFalse)
					So(attempt.Message, ShouldEqual, "Invalid config")
					So(attempt.Revision.ID, ShouldEqual, latestCommit.Id)
					So(attempt.ValidationResult, ShouldResembleProto, mockValidator.result)
				})
			})

			Convey("large config", func() {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)
				// Construct incompressible data which is larger than compressedContentLimit.
				incompressible := make([]byte, compressedContentLimit+1024*1024)
				_, err := rand.New(rand.NewSource(1234)).Read(incompressible)
				So(err, ShouldBeNil)
				compressed, err := gzipCompress(incompressible)
				So(err, ShouldBeNil)
				So(len(compressed) > compressedContentLimit, ShouldBeTrue)

				tarGzContent, err := buildTarGz(map[string]any{"large": incompressible})
				So(err, ShouldBeNil)
				expectedSha256 := sha256.Sum256(incompressible)
				expectedGsFileName := "configs/sha256/" + hex.EncodeToString(expectedSha256[:])
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{
					Contents: tarGzContent,
				}, nil)
				mockGsClient.EXPECT().UploadIfMissing(
					gomock.Any(), gomock.Eq(testGSBucket),
					gomock.Eq(expectedGsFileName), gomock.Any(), gomock.Any()).Return(true, nil)

				err = importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))

				So(err, ShouldBeNil)
				revKey := datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice", model.RevisionKind, latestCommit.Id)
				var files []*model.File
				So(datastore.GetAll(ctx, datastore.NewQuery(model.FileKind).Ancestor(revKey), &files), ShouldBeNil)
				So(files, ShouldHaveLength, 1)
				file := files[0]
				So(file.Path, ShouldEqual, "large")
				So(file.Content, ShouldBeNil)
				So(file.GcsURI, ShouldEqual, gs.MakePath(testGSBucket, expectedGsFileName))
			})
		})

		Convey("unhappy path", func() {
			Convey("bad config set format", func() {
				err := importer.ImportConfigSet(ctx, config.Set("bad"))
				So(err, ShouldErrLike, "Invalid config set")
			})

			Convey("project doesn't exist ", func() {
				testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
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
				So(err, ShouldErrLike, `project "unknown_proj" not exist or has no gitiles location`)
				So(ErrFatalTag.In(err), ShouldBeTrue)
			})

			Convey("no project gitiles location", func() {
				testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
					common.ProjRegistryFilePath: &cfgcommonpb.ProjectsCfg{
						Projects: []*cfgcommonpb.Project{
							{Id: "proj"},
						},
					},
				})
				err := importer.ImportConfigSet(ctx, config.MustProjectSet("proj"))
				So(err, ShouldErrLike, `project "proj" not exist or has no gitiles location`)
				So(ErrFatalTag.In(err), ShouldBeTrue)
			})

			Convey("cannot fetch logs", func() {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(nil, errors.New("gitiles internal errors"))
				err := importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))
				So(err, ShouldErrLike, "cannot fetch logs", "gitiles internal errors")
				attempt := &model.ImportAttempt{
					ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice"),
				}
				So(datastore.Get(ctx, attempt), ShouldBeNil)
				So(attempt.Success, ShouldBeFalse)
				So(attempt.Message, ShouldContainSubstring, "cannot fetch logs")
			})

			Convey("bad tar file", func() {
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
				So(datastore.Put(ctx, cfgSetBeforeImport), ShouldBeNil)
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{
					Contents: []byte("invalid .tar.gz content"),
				}, nil)

				err := importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))

				So(err, ShouldErrLike, "Failed to import services/myservice revision latest revision")
				cfgSetAfterImport := &model.ConfigSet{
					ID: config.MustServiceSet("myservice"),
				}
				attempt := &model.ImportAttempt{
					ConfigSet: datastore.KeyForObj(ctx, cfgSetAfterImport),
				}
				So(datastore.Get(ctx, cfgSetAfterImport, attempt), ShouldBeNil)

				So(cfgSetAfterImport.Location, ShouldResembleProto, cfgSetBeforeImport.Location)
				So(cfgSetAfterImport.LatestRevision.ID, ShouldEqual, cfgSetBeforeImport.LatestRevision.ID)

				So(attempt.Success, ShouldBeFalse)
				So(attempt.Message, ShouldContainSubstring, "Failed to import services/myservice revision latest revision")
			})

			Convey("failed to upload to GCS", func() {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)
				// Construct incompressible data which is larger than compressedContentLimit.
				incompressible := make([]byte, compressedContentLimit+1024*1024)
				_, err := rand.New(rand.NewSource(1234)).Read(incompressible)
				So(err, ShouldBeNil)
				compressed, err := gzipCompress(incompressible)
				So(err, ShouldBeNil)
				So(len(compressed) > compressedContentLimit, ShouldBeTrue)
				tarGzContent, err := buildTarGz(map[string]any{"large": incompressible})
				So(err, ShouldBeNil)
				expectedSha256 := sha256.Sum256(incompressible)
				expectedGsFileName := "configs/sha256/" + hex.EncodeToString(expectedSha256[:])
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{
					Contents: tarGzContent,
				}, nil)
				mockGsClient.EXPECT().UploadIfMissing(
					gomock.Any(), gomock.Eq(testGSBucket),
					gomock.Eq(expectedGsFileName), gomock.Any(), gomock.Any()).Return(false, errors.New("GCS internal error"))

				err = importer.ImportConfigSet(ctx, config.MustServiceSet("myservice"))

				So(err, ShouldErrLike, "failed to upload file", "GCS internal error")
				revKey := datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice", model.RevisionKind, latestCommit.Id)
				var files []*model.File
				So(datastore.GetAll(ctx, datastore.NewQuery(model.FileKind).Ancestor(revKey), &files), ShouldBeNil)
				So(files, ShouldHaveLength, 0)

				attempt := &model.ImportAttempt{
					ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice"),
				}
				So(datastore.Get(ctx, attempt), ShouldBeNil)
				So(attempt.Success, ShouldBeFalse)
				So(attempt.Message, ShouldContainSubstring, "failed to upload file")
			})

			Convey("validation error", func() {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)

				tarGzContent, err := buildTarGz(map[string]any{"foo.cfg": "content"})
				So(err, ShouldBeNil)
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{
					Contents: tarGzContent,
				}, nil)
				mockGsClient.EXPECT().UploadIfMissing(
					gomock.Any(), gomock.Eq(testGSBucket),
					gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)

				mockValidator.err = errors.New("something went wrong during validation")
				err = importer.ImportConfigSet(ctx, cs)
				So(err, ShouldErrLike, "something went wrong during validation")
			})
		})
	})
}

func TestReImport(t *testing.T) {
	t.Parallel()

	Convey("Reimport", t, func() {
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
		testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
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

		Convey("no ConfigSet param", func() {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			importer.Reimport(rctx)
			So(rsp.Code, ShouldEqual, http.StatusBadRequest)
			So(rsp.Body.String(), ShouldContainSubstring, "config set is not specified")
		})

		Convey("invalid config set", func() {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "ConfigSet", Value: "badCfgSet"},
			}
			importer.Reimport(rctx)
			So(rsp.Code, ShouldEqual, http.StatusBadRequest)
			So(rsp.Body.String(), ShouldContainSubstring, `invalid config set: unknown domain "badCfgSet" for config set "badCfgSet"`)
		})

		Convey("no permission", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: identity.Identity("user:random@example.com"),
				FakeDB:   authtest.NewFakeDB(),
			})
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "ConfigSet", Value: "services/myservice"},
			}
			importer.Reimport(rctx)
			So(rsp.Code, ShouldEqual, http.StatusForbidden)
			So(rsp.Body.String(), ShouldContainSubstring, `"user:random@example.com" is now allowed to reimport services/myservice`)
		})

		Convey("config set not found", func() {
			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "ConfigSet", Value: "services/myservice"},
			}
			importer.Reimport(rctx)
			So(rsp.Code, ShouldEqual, http.StatusNotFound)
			So(rsp.Body.String(), ShouldContainSubstring, `"services/myservice" is not found`)
		})

		Convey("ok", func() {
			ctx = settings.WithGlobalConfigLoc(ctx, &cfgcommonpb.GitilesLocation{
				Repo: "https://a.googlesource.com/infradata/config",
				Ref:  "refs/heads/main",
				Path: "dev-configs",
			})
			testutil.InjectConfigSet(ctx, "services/myservice", nil)
			ctl := gomock.NewController(t)
			mockGtClient := mock_gitiles.NewMockGitilesClient(ctl)
			ctx = context.WithValue(ctx, &clients.MockGitilesClientKey, mockGtClient)
			mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{}, nil)

			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "ConfigSet", Value: "services/myservice"},
			}
			importer.Reimport(rctx)

			So(rsp.Code, ShouldEqual, http.StatusOK)
		})

		Convey("internal error", func() {
			ctx = settings.WithGlobalConfigLoc(ctx, &cfgcommonpb.GitilesLocation{
				Repo: "https://a.googlesource.com/infradata/config",
				Ref:  "refs/heads/main",
				Path: "dev-configs",
			})
			testutil.InjectConfigSet(ctx, "services/myservice", nil)
			ctl := gomock.NewController(t)
			mockGtClient := mock_gitiles.NewMockGitilesClient(ctl)
			ctx = context.WithValue(ctx, &clients.MockGitilesClientKey, mockGtClient)
			mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(nil, errors.New("gitiles internal error"))

			rctx.Request = (&http.Request{}).WithContext(ctx)
			rctx.Params = httprouter.Params{
				{Key: "ConfigSet", Value: "services/myservice"},
			}
			importer.Reimport(rctx)

			So(rsp.Code, ShouldEqual, http.StatusInternalServerError)
			So(rsp.Body.String(), ShouldContainSubstring, `error when reimporting "services/myservice"`)
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
			return nil, errors.Reason("unsupported type %T for %s", v, name).Err()
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
