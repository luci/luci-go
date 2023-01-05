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
