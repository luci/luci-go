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
	"sort"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	protoutil "go.chromium.org/luci/common/proto"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/gitiles/mock_gitiles"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/internal/taskpb"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestImportAllConfigs(t *testing.T) {
	t.Parallel()

	Convey("import all configs", t, func() {
		ctx := testutil.SetupContext()
		ctx, sch := tq.TestingContext(ctx, nil)

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := mock_gitiles.NewMockGitilesClient(ctl)
		ctx = context.WithValue(ctx, &clients.MockGitilesClientKey, mockClient)

		Convey("ok", func() {
			mockClient.EXPECT().ListFiles(gomock.Any(), protoutil.MatcherEqual(
				&gitilespb.ListFilesRequest{
					Project:    "infradata/config",
					Committish: "main",
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
				err := ImportAllConfigs(ctx)
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
				err := ImportAllConfigs(ctx)
				So(err, ShouldBeNil)
				cfgSetsInQueue := getCfgSetsInTaskQueue(sch)
				So(cfgSetsInQueue, ShouldResemble, []string{"projects/proj1", "services/service1", "services/service2"})
			})

			Convey("delete stale config set", func() {
				stale := &model.ConfigSet{ID: config.MustServiceSet("stale")}
				So(datastore.Put(ctx, stale), ShouldBeNil)
				err := ImportAllConfigs(ctx)
				So(err, ShouldBeNil)
				So(datastore.Get(ctx, stale), ShouldEqual, datastore.ErrNoSuchEntity)
			})
		})

		Convey("error", func() {
			Convey("gitiles", func() {
				mockClient.EXPECT().ListFiles(gomock.Any(), gomock.Any()).Return(nil, errors.New("gitiles error"))
				err := ImportAllConfigs(ctx)
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
				So(ImportAllConfigs(ctx), ShouldErrLike, `failed to load project config sets: failed to unmarshal file "projects.cfg": proto`)
			})
		})
	})
}

func TestImportConfigSet(t *testing.T) {
	t.Parallel()

	Convey("import single ConfigSet", t, func() {
		ctx := testutil.SetupContext()
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockGtClient := mock_gitiles.NewMockGitilesClient(ctl)
		ctx = context.WithValue(ctx, &clients.MockGitilesClientKey, mockGtClient)
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

		Convey("happy path", func() {
			Convey("success import", func() {
				mockGtClient.EXPECT().Log(gomock.Any(), protoutil.MatcherEqual(
					&gitilespb.LogRequest{
						Project:    "infradata/config",
						Committish: "main",
						Path:       "dev-configs/myservice",
						PageSize:   1,
					},
				)).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)
				tarGzContent, err := buildTarGz(map[string]string{"file1": "file1 content", "sub_dir/file2": "file2 content", "sub_dir/": "", "empty_file": ""})
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

				err = ImportConfigSet(ctx, config.MustServiceSet("myservice"))
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
							Repo: "https://chrome-internal.googlesource.com/infradata/config",
							Ref:  "main",
							Path: "dev-configs/myservice",
						},
					},
				})
				So(cfgSet.LatestRevision.Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://chrome-internal.googlesource.com/infradata/config",
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
				So(files[0].Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://chrome-internal.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice/empty_file",
						},
					},
				})
				files[0].Location = nil
				expectedSha256 := sha256.Sum256([]byte(""))
				expectedContent0, err := gzipCompress([]byte(""))
				So(err, ShouldBeNil)
				So(files[0], ShouldResemble, &model.File{
					Path:        "empty_file",
					Revision:    revKey,
					CreateTime:  datastore.RoundTime(clock.Now(ctx).UTC()),
					Content:     expectedContent0,
					ContentHash: "sha256:" + hex.EncodeToString(expectedSha256[:]),
				})
				So(files[1].Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://chrome-internal.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice/file1",
						},
					},
				})
				files[1].Location = nil
				expectedSha256 = sha256.Sum256([]byte("file1 content"))
				expectedContent1, err := gzipCompress([]byte("file1 content"))
				So(err, ShouldBeNil)
				So(files[1], ShouldResemble, &model.File{
					Path:        "file1",
					Revision:    revKey,
					CreateTime:  datastore.RoundTime(clock.Now(ctx).UTC()),
					Content:     expectedContent1,
					ContentHash: "sha256:" + hex.EncodeToString(expectedSha256[:]),
				})
				So(files[2].Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://chrome-internal.googlesource.com/infradata/config",
							Ref:  latestCommit.Id,
							Path: "dev-configs/myservice/sub_dir/file2",
						},
					},
				})
				files[2].Location = nil
				expectedSha256 = sha256.Sum256([]byte("file2 content"))
				expectedContent2, err := gzipCompress([]byte("file2 content"))
				So(err, ShouldBeNil)
				So(files[2], ShouldResemble, &model.File{
					Path:        "sub_dir/file2",
					Revision:    revKey,
					CreateTime:  datastore.RoundTime(clock.Now(ctx).UTC()),
					Content:     expectedContent2,
					ContentHash: "sha256:" + hex.EncodeToString(expectedSha256[:]),
				})

				So(attempt.Revision.Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://chrome-internal.googlesource.com/infradata/config",
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
							Repo: "https://chrome-internal.googlesource.com/infradata/config",
							Ref:  "main",
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

				err := ImportConfigSet(ctx, config.MustServiceSet("myservice"))

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
						Committish: "main",
						Path:       "dev-configs/myservice",
						PageSize:   1,
					},
				)).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{}, nil)

				err := ImportConfigSet(ctx, config.MustServiceSet("myservice"))

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
							Repo: "https://chrome-internal.googlesource.com/infradata/config",
							Ref:  "main",
							Path: "dev-configs/myservice",
						},
					},
				})
				So(cfgSet.LatestRevision.Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://chrome-internal.googlesource.com/infradata/config",
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
								Repo: "https://chrome-internal.googlesource.com/infradata/config",
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
						Committish: "main",
						Path:       "dev-configs/myservice",
						PageSize:   1,
					},
				)).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{latestCommit},
				}, nil)
				mockGtClient.EXPECT().Archive(gomock.Any(), gomock.Any()).Return(&gitilespb.ArchiveResponse{}, nil)

				err := ImportConfigSet(ctx, config.MustServiceSet("myservice"))

				So(err, ShouldBeNil)
				cfgSetAfterImport := &model.ConfigSet{
					ID: config.MustServiceSet("myservice"),
				}
				So(datastore.Get(ctx, cfgSetAfterImport), ShouldBeNil)
				So(cfgSetAfterImport.Location.GetGitilesLocation().Ref, ShouldNotEqual, cfgSetBeforeImport.Location.GetGitilesLocation().Ref)
				So(cfgSetAfterImport.Location, ShouldResembleProto, &cfgcommonpb.Location{
					Location: &cfgcommonpb.Location_GitilesLocation{
						GitilesLocation: &cfgcommonpb.GitilesLocation{
							Repo: "https://chrome-internal.googlesource.com/infradata/config",
							Ref:  "main",
							Path: "dev-configs/myservice",
						},
					},
				})
			})

			Convey("no logs", func() {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(&gitilespb.LogResponse{
					Log: []*git.Commit{},
				}, nil)
				err := ImportConfigSet(ctx, config.MustServiceSet("myservice"))
				So(err, ShouldBeNil)
				attempt := &model.ImportAttempt{
					ConfigSet: datastore.MakeKey(ctx, model.ConfigSetKind, "services/myservice"),
				}
				So(datastore.Get(ctx, attempt), ShouldBeNil)
				So(attempt.Success, ShouldBeTrue)
				So(attempt.Message, ShouldContainSubstring, "no commit logs")
			})
		})

		Convey("unhappy path", func() {
			Convey("bad config set format", func() {
				err := ImportConfigSet(ctx, config.Set("bad"))
				So(err, ShouldErrLike, "Invalid config set")
			})

			Convey("project doesn't exist ", func() {
				testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
					projRegistryFilePath: &cfgcommonpb.ProjectsCfg{
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
				err := ImportConfigSet(ctx, config.MustProjectSet("unknown_proj"))
				So(err, ShouldErrLike, `project "unknown_proj" not exist or has no gitiles location`)
				So(ErrFatalTag.In(err), ShouldBeTrue)
			})

			Convey("no project gitiles location", func() {
				testutil.InjectSelfConfigs(ctx, map[string]proto.Message{
					projRegistryFilePath: &cfgcommonpb.ProjectsCfg{
						Projects: []*cfgcommonpb.Project{
							{Id: "proj"},
						},
					},
				})
				err := ImportConfigSet(ctx, config.MustProjectSet("proj"))
				So(err, ShouldErrLike, `project "proj" not exist or has no gitiles location`)
				So(ErrFatalTag.In(err), ShouldBeTrue)
			})

			Convey("cannot fetch logs", func() {
				mockGtClient.EXPECT().Log(gomock.Any(), gomock.Any()).Return(nil, errors.New("gitiles internal errors"))
				err := ImportConfigSet(ctx, config.MustServiceSet("myservice"))
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
							Repo: "https://chrome-internal.googlesource.com/infradata/config",
							Ref:  "main",
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

				err := ImportConfigSet(ctx, config.MustServiceSet("myservice"))

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
		})
	})
}

func buildTarGz(raw map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, body := range raw {
		hdr := &tar.Header{
			Name:     name,
			Size:     int64(len([]byte(body))),
			Typeflag: tar.TypeReg,
		}
		if strings.HasSuffix(name, "/") {
			hdr.Typeflag = tar.TypeDir
		}

		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write([]byte(body)); err != nil {
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
