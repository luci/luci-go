// Copyright 2018 The LUCI Authors.
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

package cipd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/client/cipd/local"
	"go.chromium.org/luci/cipd/client/cipd/platform"
	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/cipd/common"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

////////////////////////////////////////////////////////////////////////////////
// ACL related calls.

func TestFetchACL(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		Convey("Works", func() {
			repo.expect(rpcCall{
				method: "GetInheritedPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				out: &api.InheritedPrefixMetadata{
					PerPrefixMetadata: []*api.PrefixMetadata{
						{
							Prefix: "a",
							Acls: []*api.PrefixMetadata_ACL{
								{Role: api.Role_READER, Principals: []string{"group:a"}},
							},
						},
					},
				},
			})

			acl, err := client.FetchACL(ctx, "a/b/c")
			So(err, ShouldBeNil)

			So(acl, ShouldResemble, []PackageACL{
				{
					PackagePath: "a",
					Role:        "READER",
					Principals:  []string{"group:a"},
				},
			})
		})

		Convey("Bad prefix", func() {
			_, err := client.FetchACL(ctx, "a/b////")
			So(err, ShouldErrLike, "invalid package prefix")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "GetInheritedPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.FetchACL(ctx, "a/b/c")
			So(err, ShouldErrLike, "blah error")
		})
	})
}

func TestModifyACL(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		Convey("Modifies existing", func() {
			repo.expect(rpcCall{
				method: "GetPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a"},
				out: &api.PrefixMetadata{
					Prefix: "a",
					Acls: []*api.PrefixMetadata_ACL{
						{Role: api.Role_READER, Principals: []string{"group:a"}},
					},
					Fingerprint: "abc",
				},
			})
			repo.expect(rpcCall{
				method: "UpdatePrefixMetadata",
				in: &api.PrefixMetadata{
					Prefix:      "a",
					Fingerprint: "abc",
				},
				out: &api.PrefixMetadata{}, // ignored
			})

			So(client.ModifyACL(ctx, "a", []PackageACLChange{
				{Action: RevokeRole, Role: "READER", Principal: "group:a"},
			}), ShouldBeNil)
		})

		Convey("Creates new", func() {
			repo.expect(rpcCall{
				method: "GetPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a"},
				err:    status.Errorf(codes.NotFound, "no metadata"),
			})
			repo.expect(rpcCall{
				method: "UpdatePrefixMetadata",
				in: &api.PrefixMetadata{
					Prefix: "a",
					Acls: []*api.PrefixMetadata_ACL{
						{Role: api.Role_READER, Principals: []string{"group:a"}},
					},
				},
				out: &api.PrefixMetadata{}, // ignored
			})

			So(client.ModifyACL(ctx, "a", []PackageACLChange{
				{Action: GrantRole, Role: "READER", Principal: "group:a"},
			}), ShouldBeNil)
		})

		Convey("Noop update", func() {
			repo.expect(rpcCall{
				method: "GetPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a"},
				out: &api.PrefixMetadata{
					Prefix: "a",
					Acls: []*api.PrefixMetadata_ACL{
						{Role: api.Role_READER, Principals: []string{"group:a"}},
					},
					Fingerprint: "abc",
				},
			})

			So(client.ModifyACL(ctx, "a", []PackageACLChange{
				{Action: GrantRole, Role: "READER", Principal: "group:a"},
			}), ShouldBeNil)
		})

		someChanges := []PackageACLChange{
			{Action: RevokeRole, Role: "READER", Principal: "group:a"},
		}

		Convey("Bad prefix", func() {
			So(client.ModifyACL(ctx, "a/b////", someChanges), ShouldErrLike, "invalid package prefix")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "GetPrefixMetadata",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			So(client.ModifyACL(ctx, "a/b/c", someChanges), ShouldErrLike, "blah error")
		})
	})
}

func TestFetchRoles(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		Convey("Works", func() {
			repo.expect(rpcCall{
				method: "GetRolesInPrefix",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				out: &api.RolesInPrefixResponse{
					Roles: []*api.RolesInPrefixResponse_RoleInPrefix{
						{Role: api.Role_OWNER},
						{Role: api.Role_WRITER},
						{Role: api.Role_READER},
					},
				},
			})

			roles, err := client.FetchRoles(ctx, "a/b/c")
			So(err, ShouldBeNil)
			So(roles, ShouldResemble, []string{"OWNER", "WRITER", "READER"})
		})

		Convey("Bad prefix", func() {
			_, err := client.FetchRoles(ctx, "a/b////")
			So(err, ShouldErrLike, "invalid package prefix")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "GetRolesInPrefix",
				in:     &api.PrefixRequest{Prefix: "a/b/c"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.FetchRoles(ctx, "a/b/c")
			So(err, ShouldErrLike, "blah error")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Instance upload and registration.

func TestRegisterInstance(t *testing.T) {
	t.Parallel()

	// TODO
}

////////////////////////////////////////////////////////////////////////////////
// Setting refs and tags.

func TestSetRefWhenReady(t *testing.T) {
	t.Parallel()

	// TODO
}

func TestAttachTagsWhenReady(t *testing.T) {
	t.Parallel()

	// TODO
}

////////////////////////////////////////////////////////////////////////////////
// Fetching info about packages and instances.

func TestListPackages(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		Convey("Works", func() {
			repo.expect(rpcCall{
				method: "ListPrefix",
				in: &api.ListPrefixRequest{
					Prefix:        "a/b/c",
					Recursive:     true,
					IncludeHidden: true,
				},
				out: &api.ListPrefixResponse{
					Packages: []string{"a/b/c/d/pkg1", "a/b/c/d/pkg2"},
					Prefixes: []string{"a/b/c/d", "a/b/c/e", "a/b/c/e/f"},
				},
			})

			out, err := client.ListPackages(ctx, "a/b/c", true, true)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []string{
				"a/b/c/d/",
				"a/b/c/d/pkg1",
				"a/b/c/d/pkg2",
				"a/b/c/e/",
				"a/b/c/e/f/",
			})
		})

		Convey("Bad prefix", func() {
			_, err := client.ListPackages(ctx, "a/b////", true, true)
			So(err, ShouldErrLike, "invalid package prefix")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "ListPrefix",
				in:     &api.ListPrefixRequest{Prefix: "a/b/c"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.ListPackages(ctx, "a/b/c", false, false)
			So(err, ShouldErrLike, "blah error")
		})
	})
}

func TestSearchInstances(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		Convey("Works", func() {
			repo.expect(rpcCall{
				method: "SearchInstances",
				in: &api.SearchInstancesRequest{
					Package: "a/b",
					Tags: []*api.Tag{
						{Key: "k1", Value: "v1"},
						{Key: "k2", Value: "v2"},
					},
					PageSize: 1000,
				},
				out: &api.SearchInstancesResponse{
					Instances: []*api.Instance{
						{
							Package: "a/b",
							Instance: &api.ObjectRef{
								HashAlgo:  api.HashAlgo_SHA1,
								HexDigest: strings.Repeat("0", 40),
							},
						},
						{
							Package: "a/b",
							Instance: &api.ObjectRef{
								HashAlgo:  api.HashAlgo_SHA1,
								HexDigest: strings.Repeat("1", 40),
							},
						},
					},
					NextPageToken: "blah", // ignored for now
				},
			})

			out, err := client.SearchInstances(ctx, "a/b", []string{"k1:v1", "k2:v2"})
			So(err, ShouldBeNil)
			So(out, ShouldResemble, common.PinSlice{
				{PackageName: "a/b", InstanceID: strings.Repeat("0", 40)},
				{PackageName: "a/b", InstanceID: strings.Repeat("1", 40)},
			})
		})

		Convey("Bad package name", func() {
			_, err := client.SearchInstances(ctx, "a/b////", []string{"k:v"})
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("No tags", func() {
			_, err := client.SearchInstances(ctx, "a/b", nil)
			So(err, ShouldErrLike, "at least one tag is required")
		})

		Convey("Bad tag", func() {
			_, err := client.SearchInstances(ctx, "a/b", []string{"bad_tag"})
			So(err, ShouldErrLike, "doesn't look like a tag")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "SearchInstances",
				in: &api.SearchInstancesRequest{
					Package: "a/b",
					Tags: []*api.Tag{
						{Key: "k", Value: "v"},
					},
					PageSize: 1000,
				},
				err: status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.SearchInstances(ctx, "a/b", []string{"k:v"})
			So(err, ShouldErrLike, "blah error")
		})
	})
}

func TestListInstances(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		fakeApiInst := func(id string) *api.Instance {
			return &api.Instance{
				Package: "a/b",
				Instance: &api.ObjectRef{
					HashAlgo:  api.HashAlgo_SHA1,
					HexDigest: strings.Repeat(id, 40),
				},
			}
		}

		fakeInstInfo := func(id string) InstanceInfo {
			return InstanceInfo{
				Pin: common.Pin{
					PackageName: "a/b",
					InstanceID:  strings.Repeat(id, 40),
				},
			}
		}

		Convey("Works", func() {
			repo.expect(rpcCall{
				method: "ListInstances",
				in: &api.ListInstancesRequest{
					Package:  "a/b",
					PageSize: 2,
				},
				out: &api.ListInstancesResponse{
					Instances:     []*api.Instance{fakeApiInst("0"), fakeApiInst("1")},
					NextPageToken: "page_tok",
				},
			})
			repo.expect(rpcCall{
				method: "ListInstances",
				in: &api.ListInstancesRequest{
					Package:   "a/b",
					PageSize:  2,
					PageToken: "page_tok",
				},
				out: &api.ListInstancesResponse{
					Instances: []*api.Instance{fakeApiInst("2")},
				},
			})

			enum, err := client.ListInstances(ctx, "a/b")
			So(err, ShouldBeNil)

			all := []InstanceInfo{}
			for {
				batch, err := enum.Next(ctx, 2)
				So(err, ShouldBeNil)
				if len(batch) == 0 {
					break
				}
				all = append(all, batch...)
			}

			So(all, ShouldResemble, []InstanceInfo{
				fakeInstInfo("0"),
				fakeInstInfo("1"),
				fakeInstInfo("2"),
			})
		})

		Convey("Bad package name", func() {
			_, err := client.ListInstances(ctx, "a/b////")
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "ListInstances",
				in: &api.ListInstancesRequest{
					Package:  "a/b",
					PageSize: 100,
				},
				err: status.Errorf(codes.PermissionDenied, "blah error"),
			})
			enum, err := client.ListInstances(ctx, "a/b")
			So(err, ShouldBeNil)
			_, err = enum.Next(ctx, 100)
			So(err, ShouldErrLike, "blah error")
		})
	})
}

func TestFetchPackageRefs(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		Convey("Works", func() {
			repo.expect(rpcCall{
				method: "ListRefs",
				in:     &api.ListRefsRequest{Package: "a/b"},
				out: &api.ListRefsResponse{
					Refs: []*api.Ref{
						{
							Name: "r1",
							Instance: &api.ObjectRef{
								HashAlgo:  api.HashAlgo_SHA1,
								HexDigest: strings.Repeat("0", 40),
							},
						},
						{
							Name: "r2",
							Instance: &api.ObjectRef{
								HashAlgo:  api.HashAlgo_SHA1,
								HexDigest: strings.Repeat("1", 40),
							},
						},
					},
				},
			})

			out, err := client.FetchPackageRefs(ctx, "a/b")
			So(err, ShouldBeNil)
			So(out, ShouldResemble, []RefInfo{
				{Ref: "r1", InstanceID: strings.Repeat("0", 40)},
				{Ref: "r2", InstanceID: strings.Repeat("1", 40)},
			})
		})

		Convey("Bad package name", func() {
			_, err := client.FetchPackageRefs(ctx, "a/b////")
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "ListRefs",
				in:     &api.ListRefsRequest{Package: "a/b"},
				err:    status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.FetchPackageRefs(ctx, "a/b")
			So(err, ShouldErrLike, "blah error")
		})
	})
}

func TestDescribeInstance(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		pin := common.Pin{
			PackageName: "a/b",
			InstanceID:  strings.Repeat("0", 40),
		}

		Convey("Works", func() {
			repo.expect(rpcCall{
				method: "DescribeInstance",
				in: &api.DescribeInstanceRequest{
					Package: "a/b",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: pin.InstanceID,
					},
					DescribeRefs: true,
					DescribeTags: true,
				},
				out: &api.DescribeInstanceResponse{
					Instance: &api.Instance{
						Package: "a/b",
						Instance: &api.ObjectRef{
							HashAlgo:  api.HashAlgo_SHA1,
							HexDigest: pin.InstanceID,
						},
					},
				},
			})

			desc, err := client.DescribeInstance(ctx, pin, &DescribeInstanceOpts{
				DescribeRefs: true,
				DescribeTags: true,
			})
			So(err, ShouldBeNil)
			So(desc, ShouldResemble, &InstanceDescription{
				InstanceInfo: InstanceInfo{Pin: pin},
			})
		})

		Convey("Bad pin", func() {
			_, err := client.DescribeInstance(ctx, common.Pin{
				PackageName: "a/b////",
			}, nil)
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "DescribeInstance",
				in: &api.DescribeInstanceRequest{
					Package: "a/b",
					Instance: &api.ObjectRef{
						HashAlgo:  api.HashAlgo_SHA1,
						HexDigest: pin.InstanceID,
					},
				},
				err: status.Errorf(codes.PermissionDenied, "blah error"),
			})
			_, err := client.DescribeInstance(ctx, pin, nil)
			So(err, ShouldErrLike, "blah error")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Version resolution (including tag cache).

func TestResolveVersion(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func(c C) {
		ctx := context.Background()
		client, _, repo, _ := mockedCipdClient(c)

		expectedPin := common.Pin{
			PackageName: "a/b",
			InstanceID:  strings.Repeat("0", 40),
		}
		resolvedInst := &api.Instance{
			Package: "a/b",
			Instance: &api.ObjectRef{
				HashAlgo:  api.HashAlgo_SHA1,
				HexDigest: expectedPin.InstanceID,
			},
		}

		Convey("Resolves ref", func() {
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b",
					Version: "latest",
				},
				out: resolvedInst,
			})
			pin, err := client.ResolveVersion(ctx, "a/b", "latest")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, expectedPin)
		})

		Convey("Skips resolving instance ID", func() {
			pin, err := client.ResolveVersion(ctx, "a/b", expectedPin.InstanceID)
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, expectedPin)
		})

		Convey("Resolves tag (no tag cache)", func() {
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b",
					Version: "k:v",
				},
				out: resolvedInst,
			})
			pin, err := client.ResolveVersion(ctx, "a/b", "k:v")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, expectedPin)
		})

		Convey("Resolves tag (with tag cache)", func() {
			setupTagCache(client, "", c)

			// Only one RPC, even though we did two ResolveVersion calls.
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b",
					Version: "k:v",
				},
				out: resolvedInst,
			})

			// Cache miss.
			pin, err := client.ResolveVersion(ctx, "a/b", "k:v")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, expectedPin)

			// Cache hit.
			pin, err = client.ResolveVersion(ctx, "a/b", "k:v")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, expectedPin)
		})

		Convey("Bad package name", func() {
			_, err := client.ResolveVersion(ctx, "a/b////", "latest")
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Bad version identifier", func() {
			_, err := client.ResolveVersion(ctx, "a/b", "???")
			So(err, ShouldErrLike, "bad version")
		})

		Convey("Error response", func() {
			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: "a/b",
					Version: "k:v",
				},
				err: status.Errorf(codes.FailedPrecondition, "conflict"),
			})
			_, err := client.ResolveVersion(ctx, "a/b", "k:v")
			So(err, ShouldErrLike, "conflict")
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Instance fetching (including instance cache).

func TestFetchInstance(t *testing.T) {
	t.Parallel()

	// TODO
}

////////////////////////////////////////////////////////////////////////////////
// Instance installation.

func TestEnsurePackage(t *testing.T) {
	t.Parallel()

	// TODO
}

////////////////////////////////////////////////////////////////////////////////
// Client self-update.

func TestMaybeUpdateClient(t *testing.T) {
	t.Parallel()

	clientPackage, err := template.DefaultExpander().Expand(ClientPackage)
	if err != nil {
		panic(err)
	}
	clientFileName := "cipd"
	if platform.CurrentOS() == "windows" {
		clientFileName = "cipd.exe"
	}

	ctx := context.Background()

	Convey("With temp dir", t, func(c C) {
		tempDir, err := ioutil.TempDir("", "cipd_tag_cache")
		c.So(err, ShouldBeNil)
		c.Reset(func() {
			os.RemoveAll(tempDir)
		})

		clientOpts, _, repo, storage := mockedClientOpts(c)

		clientPkgRef := &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
			HexDigest: strings.Repeat("a", 64),
		}
		clientPin := common.Pin{
			PackageName: clientPackage,
			InstanceID:  common.ObjectRefToInstanceID(clientPkgRef),
		}

		clientBin := filepath.Join(tempDir, clientFileName)

		caclObjRef := func(body string, algo api.HashAlgo) *api.ObjectRef {
			h := common.MustNewHash(algo)
			h.Write([]byte(body))
			return &api.ObjectRef{
				HashAlgo:  algo,
				HexDigest: common.HexDigest(h),
			}
		}

		writeFile := func(path, body string) {
			So(ioutil.WriteFile(path, []byte(body), 0777), ShouldBeNil)
		}

		readFile := func(path string) string {
			body, err := ioutil.ReadFile(path)
			So(err, ShouldBeNil)
			return string(body)
		}

		Convey("MaybeUpdateClient updates outdated client, warming caches", func() {
			storage.putStored("http://example.com/client_bin", "up-to-date")

			upToDateSHA256Ref := caclObjRef("up-to-date", api.HashAlgo_SHA256)
			upToDateSHA1Ref := caclObjRef("up-to-date", api.HashAlgo_SHA256)
			upToDateUnsupRef := &api.ObjectRef{HashAlgo: 555, HexDigest: strings.Repeat("f", 66)}

			writeFile(clientBin, "outdated")

			repo.expect(rpcCall{
				method: "ResolveVersion",
				in: &api.ResolveVersionRequest{
					Package: clientPackage,
					Version: "git:deadbeef",
				},
				out: &api.Instance{
					Package:  clientPackage,
					Instance: clientPkgRef,
				},
			})
			repo.expect(rpcCall{
				method: "DescribeClient",
				in: &api.DescribeClientRequest{
					Package:  clientPackage,
					Instance: clientPkgRef,
				},
				out: &api.DescribeClientResponse{
					Instance: &api.Instance{
						Package:  clientPackage,
						Instance: clientPkgRef,
					},
					ClientRef: upToDateSHA256Ref,
					ClientBinary: &api.ObjectURL{
						SignedUrl: "http://example.com/client_bin",
					},
					ClientSize: 10000,
					LegacySha1: upToDateSHA1Ref.HexDigest,
					ClientRefAliases: []*api.ObjectRef{
						upToDateSHA1Ref,
						upToDateSHA256Ref,
						upToDateUnsupRef,
					},
				},
			})

			// Should update 'outdated' to 'up-to-date' and warm up the tag cache.
			pin, err := MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin)
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, clientPin)

			// Yep, updated.
			So(readFile(clientBin), ShouldEqual, "up-to-date")

			// Also drops .cipd_version file.
			verFile := filepath.Join(tempDir, ".versions", clientFileName+".cipd_version")
			So(readFile(verFile), ShouldEqual,
				fmt.Sprintf(`{"package_name":"%s","instance_id":"%s"}`, clientPin.PackageName, clientPin.InstanceID))

			// And the second update call does nothing and hits no RPCs, since the
			// client is up-to-date and the tag cache is warm.
			pin, err = MaybeUpdateClient(ctx, clientOpts, "git:deadbeef", clientBin)
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, clientPin)
		})
	})
}

////////////////////////////////////////////////////////////////////////////////
// Batch ops.

// TODO

////////////////////////////////////////////////////////////////////////////////

func mockedClientOpts(c C) (ClientOptions, *mockedStorageClient, *mockedRepoClient, *mockedStorage) {
	cas := &mockedStorageClient{}
	cas.C(c)
	repo := &mockedRepoClient{}
	repo.C(c)

	// When the test case ends, make sure all expected RPCs were called.
	c.Reset(func() {
		cas.assertAllCalled()
		repo.assertAllCalled()
	})

	storage := &mockedStorage{}

	return ClientOptions{
		ServiceURL:  "https://service.example.com",
		casMock:     cas,
		repoMock:    repo,
		storageMock: storage,
	}, cas, repo, storage
}

func mockedCipdClient(c C) (*clientImpl, *mockedStorageClient, *mockedRepoClient, *mockedStorage) {
	opts, cas, repo, storage := mockedClientOpts(c)
	client, err := NewClient(opts)
	c.So(err, ShouldBeNil)
	impl := client.(*clientImpl)
	return impl, cas, repo, storage
}

func setupTagCache(cl *clientImpl, tempDir string, c C) {
	So(cl.tagCache, ShouldBeNil)
	if tempDir == "" {
		var err error
		tempDir, err = ioutil.TempDir("", "cipd_tag_cache")
		c.So(err, ShouldBeNil)
		c.Reset(func() {
			os.RemoveAll(tempDir)
		})
	}
	cl.tagCache = internal.NewTagCache(local.NewFileSystem(tempDir, ""), "service.example.com")
}
