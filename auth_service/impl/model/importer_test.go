// Copyright 2022 The LUCI Authors.
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

package model

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func testGroupImporterConfig() *GroupImporterConfig {
	return &GroupImporterConfig{
		Kind: "GroupImporterConfig",
		ID:   "config",
		ConfigProto: `
			# Schema for this file:
			# https://luci-config.appspot.com/schemas/services/chrome-infra-auth:imports.cfg
			# See GroupImporterConfig message.

			# Groups pushed by //depot/google3/googleclient/chrome/infra/groups_push_cron/
			#
			# To add new groups, see README.md there.

			tarball_upload {
			name: "test_groups.tar.gz"
			authorized_uploader: "test-push-cron@system.example.com"
			systems: "tst"
			}

			tarball_upload {
			name: "example_groups.tar.gz"
			authorized_uploader: "another-push-cron@system.example.com"
			systems: "examp"
			}
		`,
		ConfigRevision: []byte("some-config-revision"),
		ModifiedBy:     "some-user@example.com",
		ModifiedTS:     testModifiedTS,
	}
}
func TestGroupImporterConfigModel(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())

	Convey("testing GetGroupImporterConfig", t, func() {
		groupCfg := testGroupImporterConfig()

		_, err := GetGroupImporterConfig(ctx)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		So(datastore.Put(ctx, groupCfg), ShouldBeNil)

		actual, err := GetGroupImporterConfig(ctx)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, groupCfg)
	})
}

func TestLoadGroupFile(t *testing.T) {
	t.Parallel()
	testDomain := "example.com"

	Convey("Testing LoadGroupFile()", t, func() {
		Convey("OK", func() {
			body := strings.Join([]string{"", "b", "a", "a", ""}, "\n")
			aIdent, _ := identity.MakeIdentity(fmt.Sprintf("user:a@%s", testDomain))
			bIdent, _ := identity.MakeIdentity(fmt.Sprintf("user:b@%s", testDomain))

			actual, err := loadGroupFile(body, testDomain)
			So(err, ShouldBeNil)
			So(actual, ShouldResemble, []identity.Identity{
				aIdent,
				bIdent,
			})
		})
		Convey("bad id", func() {
			body := "bad id"
			_, err := loadGroupFile(body, testDomain)
			So(err, ShouldErrLike, `auth: bad value "bad id@example.com" for identity kind "user"`)
		})
	})
}

func TestExtractTarArchive(t *testing.T) {
	t.Parallel()
	Convey("valid tarball with skippable files", t, func() {
		expected := map[string][]byte{
			"at_root":             []byte("a\nb"),
			"ldap/ bad name":      []byte("a\nb"),
			"ldap/group-a":        []byte("a\nb"),
			"ldap/group-b":        []byte("a\nb"),
			"ldap/group-c":        []byte("a\nb"),
			"ldap/deeper/group-a": []byte("a\nb"),
			"not-ldap/group-a":    []byte("a\nb"),
		}
		bundle := buildTargz(expected)
		entries, err := extractTarArchive(bytes.NewReader(bundle))
		So(err, ShouldBeNil)
		So(entries, ShouldResemble, expected)
	})
}

func TestLoadTarball(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())

	Convey("testing loadTarball", t, func() {
		Convey("invalid tarball bad identity", func() {
			bundle := buildTargz(map[string][]byte{
				"at_root":      []byte("a\nb"),
				"ldap/group-a": []byte("a\n!!!!!!"),
			})
			_, err := loadTarball(ctx, bytes.NewReader(bundle), "example.com", []string{"ldap"}, []string{"ldap/group-a", "ldap/group-b"})
			So(err, ShouldErrLike, `auth: bad value "!!!!!!@example.com" for identity kind "user"`)
		})
		Convey("valid tarball with skippable files", func() {
			bundle := buildTargz(map[string][]byte{
				"at_root":             []byte("a\nb"),
				"ldap/ bad name":      []byte("a\nb"),
				"ldap/group-a":        []byte("a\nb"),
				"ldap/group-b":        []byte("a\nb"),
				"ldap/group-c":        []byte("a\nb"),
				"ldap/deeper/group-a": []byte("a\nb"),
				"not-ldap/group-a":    []byte("a\nb"),
			})
			m, err := loadTarball(ctx, bytes.NewReader(bundle), "example.com", []string{"ldap"}, []string{"ldap/group-a", "ldap/group-b"})
			So(err, ShouldBeNil)
			aIdent, _ := identity.MakeIdentity("user:a@example.com")
			bIdent, _ := identity.MakeIdentity("user:b@example.com")
			So(m, ShouldResemble, map[string]groupBundle{
				"ldap": {
					"ldap/group-a": {
						aIdent,
						bIdent,
					},
					"ldap/group-b": {
						aIdent,
						bIdent,
					},
				},
			})
		})
	})
}

func TestIngestTarball(t *testing.T) {
	t.Parallel()
	ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
		Identity: "user:test-push-cron@system.example.com",
	})

	cfg := testGroupImporterConfig()

	bundle := buildTargz(map[string][]byte{
		"at_root":            []byte("a\nb"),
		"tst/ bad name":      []byte("a\nb"),
		"tst/group-a":        []byte("a@example.com\nb@example.test.com"),
		"tst/group-b":        []byte("a@example.com"),
		"tst/group-c":        []byte("a@example.com\nc@test-example.com"),
		"tst/deeper/group-a": []byte("a\nb"),
		"not-tst/group-a":    []byte("a\nb"),
	})

	Convey("testing ingestTarball", t, func() {
		Convey("unknown", func() {
			datastore.Put(ctx, cfg)
			_, err := ingestTarball(ctx, "zzz", nil)
			So(err, ShouldErrLike, "entry not found in tarball upload names")
		})
		Convey("unauthorized", func() {
			badAuthCtx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
			})
			_, err := ingestTarball(badAuthCtx, "test_groups.tar.gz", bytes.NewReader(bundle))
			So(err, ShouldErrLike, `"someone@example.com" is not an authorized uploader`)
		})
		Convey("not configured", func() {
			badCtx := memory.Use(context.Background())
			_, err := ingestTarball(badCtx, "", nil)
			So(err, ShouldErrLike, datastore.ErrNoSuchEntity)
		})
		Convey("happy", func() {
			groups, err := ingestTarball(ctx, "test_groups.tar.gz", bytes.NewReader(bundle))
			So(err, ShouldBeNil)
			aIdent, _ := identity.MakeIdentity("user:a@example.com")
			bIdent, _ := identity.MakeIdentity("user:b@example.test.com")
			cIdent, _ := identity.MakeIdentity("user:c@test-example.com")
			So(groups, ShouldResemble, map[string]groupBundle{
				"tst": {
					"tst/group-a": {
						aIdent,
						bIdent,
					},
					"tst/group-b": {
						aIdent,
					},
					"tst/group-c": {
						aIdent,
						cIdent,
					},
				},
			})
		})
	})
}

func buildTargz(files map[string][]byte) []byte {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for name, body := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0600,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil
		}
		if _, err := tw.Write(body); err != nil {
			return nil
		}
	}
	if err := tw.Flush(); err != nil {
		return nil
	}
	if err := tw.Close(); err != nil {
		return nil
	}
	if err := gzw.Flush(); err != nil {
		return nil
	}
	if err := gzw.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}
