// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cipd

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logging/gologger"

	. "github.com/luci/luci-go/cipd/client/cipd/common"
	"github.com/luci/luci-go/cipd/client/cipd/internal"
	"github.com/luci/luci-go/cipd/client/cipd/local"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUploadToCAS(t *testing.T) {
	ctx := makeTestContext()

	Convey("UploadToCAS full flow", t, func(c C) {
		client := mockClient(c, "", []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/upload/SHA1/abc",
				Reply:  `{"status":"SUCCESS","upload_session_id":"12345","upload_url":"http://localhost"}`,
			},
			{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/finalize/12345",
				Reply:  `{"status":"VERIFYING"}`,
			},
			{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/finalize/12345",
				Reply:  `{"status":"PUBLISHED"}`,
			},
		})
		client.storage = &mockedStorage{c, nil}
		err := client.UploadToCAS(ctx, "abc", nil, nil, time.Minute)
		So(err, ShouldBeNil)
	})

	Convey("UploadToCAS timeout", t, func(c C) {
		// Append a bunch of "still verifying" responses at the end.
		calls := []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/upload/SHA1/abc",
				Reply:  `{"status":"SUCCESS","upload_session_id":"12345","upload_url":"http://localhost"}`,
			},
		}
		for i := 0; i < 19; i++ {
			calls = append(calls, expectedHTTPCall{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/finalize/12345",
				Reply:  `{"status":"VERIFYING"}`,
			})
		}
		client := mockClient(c, "", calls)
		client.storage = &mockedStorage{c, nil}
		err := client.UploadToCAS(ctx, "abc", nil, nil, time.Minute)
		So(err, ShouldResemble, ErrFinalizationTimeout)
	})
}

func TestResolveVersion(t *testing.T) {
	ctx := makeTestContext()

	Convey("ResolveVersion works", t, func(c C) {
		client := mockClient(c, "", []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/instance/resolve",
				Query: url.Values{
					"package_name": []string{"pkgname"},
					"version":      []string{"tag_key:value"},
				},
				Reply: `{
					"status": "SUCCESS",
					"instance_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				}`,
			},
		})
		pin, err := client.ResolveVersion(ctx, "pkgname", "tag_key:value")
		So(err, ShouldBeNil)
		So(pin, ShouldResemble, Pin{
			PackageName: "pkgname",
			InstanceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		})
	})

	Convey("ResolveVersion with instance ID", t, func(c C) {
		// No calls to the backend expected.
		client := mockClient(c, "", nil)
		pin, err := client.ResolveVersion(ctx, "pkgname", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		So(err, ShouldBeNil)
		So(pin, ShouldResemble, Pin{
			PackageName: "pkgname",
			InstanceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		})
	})

	Convey("ResolveVersion bad package name", t, func(c C) {
		client := mockClient(c, "", nil)
		_, err := client.ResolveVersion(ctx, "bad package", "tag_key:value")
		So(err, ShouldNotBeNil)
	})

	Convey("ResolveVersion bad version", t, func(c C) {
		client := mockClient(c, "", nil)
		_, err := client.ResolveVersion(ctx, "pkgname", "BAD_TAG:")
		So(err, ShouldNotBeNil)
	})
}

func TestRegisterInstance(t *testing.T) {
	ctx := makeTestContext()

	Convey("Mocking a package instance", t, func() {
		// Build an empty package to be uploaded.
		out := bytes.Buffer{}
		err := local.BuildInstance(ctx, local.BuildInstanceOptions{
			Input:            []local.File{},
			Output:           &out,
			PackageName:      "testing",
			CompressionLevel: 5,
		})
		So(err, ShouldBeNil)

		// Open it for reading.
		inst, err := local.OpenInstance(ctx, bytes.NewReader(out.Bytes()), "")
		So(err, ShouldBeNil)
		defer inst.Close()

		Convey("RegisterInstance full flow", func(c C) {
			client := mockClient(c, "", []expectedHTTPCall{
				{
					Method: "POST",
					Path:   "/_ah/api/repo/v1/instance",
					Query: url.Values{
						"instance_id":  []string{inst.Pin().InstanceID},
						"package_name": []string{inst.Pin().PackageName},
					},
					Reply: `{
						"status": "UPLOAD_FIRST",
						"upload_session_id": "12345",
						"upload_url": "http://localhost"
					}`,
				},
				{
					Method: "POST",
					Path:   "/_ah/api/cas/v1/finalize/12345",
					Reply:  `{"status":"PUBLISHED"}`,
				},
				{
					Method: "POST",
					Path:   "/_ah/api/repo/v1/instance",
					Query: url.Values{
						"instance_id":  []string{inst.Pin().InstanceID},
						"package_name": []string{inst.Pin().PackageName},
					},
					Reply: `{
						"status": "REGISTERED",
						"instance": {
							"registered_by": "user:a@example.com",
							"registered_ts": "0"
						}
					}`,
				},
			})
			client.storage = &mockedStorage{c, nil}
			err = client.RegisterInstance(ctx, inst, time.Minute)
			So(err, ShouldBeNil)
		})

		Convey("RegisterInstance already registered", func(c C) {
			client := mockClient(c, "", []expectedHTTPCall{
				{
					Method: "POST",
					Path:   "/_ah/api/repo/v1/instance",
					Query: url.Values{
						"instance_id":  []string{inst.Pin().InstanceID},
						"package_name": []string{inst.Pin().PackageName},
					},
					Reply: `{
							"status": "ALREADY_REGISTERED",
							"instance": {
								"registered_by": "user:a@example.com",
								"registered_ts": "0"
							}
						}`,
				},
			})
			client.storage = &mockedStorage{c, nil}
			err = client.RegisterInstance(ctx, inst, time.Minute)
			So(err, ShouldBeNil)
		})
	})
}

func TestSetRefWhenReady(t *testing.T) {
	ctx := makeTestContext()

	Convey("SetRefWhenReady works", t, func(c C) {
		client := mockClient(c, "", []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/repo/v1/ref",
				Query: url.Values{
					"package_name": []string{"pkgname"},
					"ref":          []string{"some-ref"},
				},
				Body:  `{"instance_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
				Reply: `{"status": "PROCESSING_NOT_FINISHED_YET"}`,
			},
			{
				Method: "POST",
				Path:   "/_ah/api/repo/v1/ref",
				Query: url.Values{
					"package_name": []string{"pkgname"},
					"ref":          []string{"some-ref"},
				},
				Body:  `{"instance_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
				Reply: `{"status": "SUCCESS"}`,
			},
		})
		pin := Pin{
			PackageName: "pkgname",
			InstanceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		err := client.SetRefWhenReady(ctx, "some-ref", pin)
		So(err, ShouldBeNil)
	})

	Convey("SetRefWhenReady timeout", t, func(c C) {
		calls := []expectedHTTPCall{}
		for i := 0; i < 36; i++ {
			calls = append(calls, expectedHTTPCall{
				Method: "POST",
				Path:   "/_ah/api/repo/v1/ref",
				Query: url.Values{
					"package_name": []string{"pkgname"},
					"ref":          []string{"some-ref"},
				},
				Body:  `{"instance_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
				Reply: `{"status": "PROCESSING_NOT_FINISHED_YET"}`,
			})
		}
		client := mockClient(c, "", calls)
		pin := Pin{
			PackageName: "pkgname",
			InstanceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		err := client.SetRefWhenReady(ctx, "some-ref", pin)
		So(err, ShouldResemble, ErrSetRefTimeout)
	})
}

func TestAttachTagsWhenReady(t *testing.T) {
	ctx := makeTestContext()

	Convey("AttachTagsWhenReady works", t, func(c C) {
		client := mockClient(c, "", []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/repo/v1/tags",
				Query: url.Values{
					"instance_id":  []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					"package_name": []string{"pkgname"},
				},
				Body:  `{"tags":["tag1:value1"]}`,
				Reply: `{"status": "PROCESSING_NOT_FINISHED_YET"}`,
			},
			{
				Method: "POST",
				Path:   "/_ah/api/repo/v1/tags",
				Query: url.Values{
					"instance_id":  []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					"package_name": []string{"pkgname"},
				},
				Body:  `{"tags":["tag1:value1"]}`,
				Reply: `{"status": "SUCCESS"}`,
			},
		})
		pin := Pin{
			PackageName: "pkgname",
			InstanceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		err := client.AttachTagsWhenReady(ctx, pin, []string{"tag1:value1"})
		So(err, ShouldBeNil)
	})

	Convey("AttachTagsWhenReady timeout", t, func(c C) {
		calls := []expectedHTTPCall{}
		for i := 0; i < 36; i++ {
			calls = append(calls, expectedHTTPCall{
				Method: "POST",
				Path:   "/_ah/api/repo/v1/tags",
				Query: url.Values{
					"instance_id":  []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					"package_name": []string{"pkgname"},
				},
				Body:  `{"tags":["tag1:value1"]}`,
				Reply: `{"status": "PROCESSING_NOT_FINISHED_YET"}`,
			})
		}
		client := mockClient(c, "", calls)
		pin := Pin{
			PackageName: "pkgname",
			InstanceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		err := client.AttachTagsWhenReady(ctx, pin, []string{"tag1:value1"})
		So(err, ShouldResemble, ErrAttachTagsTimeout)
	})
}

func TestFetchInstanceInfo(t *testing.T) {
	ctx := makeTestContext()

	Convey("FetchInstanceInfo works", t, func(c C) {
		client := mockClient(c, "", []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/instance",
				Query: url.Values{
					"instance_id":  []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					"package_name": []string{"pkgname"},
				},
				Reply: `{
					"status": "SUCCESS",
					"instance": {
						"registered_by": "user:a@example.com",
						"registered_ts": "1420244414571500"
					}
				}`,
			},
		})
		pin := Pin{
			PackageName: "pkgname",
			InstanceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		info, err := client.FetchInstanceInfo(ctx, pin)
		So(err, ShouldBeNil)
		So(info, ShouldResemble, InstanceInfo{
			Pin:          pin,
			RegisteredBy: "user:a@example.com",
			RegisteredTs: UnixTime(time.Unix(0, 1420244414571500000)),
		})
	})
}

func TestFetchInstanceTags(t *testing.T) {
	ctx := makeTestContext()

	Convey("FetchInstanceTags works", t, func(c C) {
		client := mockClient(c, "", []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/tags",
				Query: url.Values{
					"instance_id":  []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					"package_name": []string{"pkgname"},
				},
				Reply: `{
					"status": "SUCCESS",
					"tags": [
						{
							"tag": "z:earlier",
							"registered_by": "user:a@example.com",
							"registered_ts": "1420244414571500"
						},
						{
							"tag": "z:later",
							"registered_by": "user:a@example.com",
							"registered_ts": "1420244414572500"
						},
						{
							"tag": "a:later",
							"registered_by": "user:a@example.com",
							"registered_ts": "1420244414572500"
						}
					]
				}`,
			},
		})
		pin := Pin{
			PackageName: "pkgname",
			InstanceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		tags, err := client.FetchInstanceTags(ctx, pin, nil)
		So(err, ShouldBeNil)
		So(tags, ShouldResemble, []TagInfo{
			{
				Tag:          "a:later",
				RegisteredBy: "user:a@example.com",
				RegisteredTs: UnixTime(time.Unix(0, 1420244414572500000)),
			},
			{
				Tag:          "z:later",
				RegisteredBy: "user:a@example.com",
				RegisteredTs: UnixTime(time.Unix(0, 1420244414572500000)),
			},
			{
				Tag:          "z:earlier",
				RegisteredBy: "user:a@example.com",
				RegisteredTs: UnixTime(time.Unix(0, 1420244414571500000)),
			},
		})
	})
}

func TestFetchInstanceRefs(t *testing.T) {
	ctx := makeTestContext()

	Convey("FetchInstanceRefs works", t, func(c C) {
		client := mockClient(c, "", []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/ref",
				Query: url.Values{
					"instance_id":  []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					"package_name": []string{"pkgname"},
				},
				Reply: `{
					"status": "SUCCESS",
					"refs": [
						{
							"ref": "ref1",
							"modified_by": "user:a@example.com",
							"modified_ts": "1420244414572500"
						},
						{
							"ref": "ref2",
							"modified_by": "user:a@example.com",
							"modified_ts": "1420244414571500"
						}
					]
				}`,
			},
		})
		pin := Pin{
			PackageName: "pkgname",
			InstanceID:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		}
		refs, err := client.FetchInstanceRefs(ctx, pin, nil)
		So(err, ShouldBeNil)
		So(refs, ShouldResemble, []RefInfo{
			{
				Ref:        "ref1",
				ModifiedBy: "user:a@example.com",
				ModifiedTs: UnixTime(time.Unix(0, 1420244414572500000)),
			},
			{
				Ref:        "ref2",
				ModifiedBy: "user:a@example.com",
				ModifiedTs: UnixTime(time.Unix(0, 1420244414571500000)),
			},
		})
	})
}

func TestFetch(t *testing.T) {
	ctx := makeTestContext()

	Convey("Mocking remote services", t, func(c C) {
		tempDir, err := ioutil.TempDir("", "cipd_test")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)

		inst := buildInstanceInMemory(ctx, "testing/package", []local.File{
			local.NewTestFile("file", "test data", false),
		})
		defer inst.Close()

		client := mockClientForFetch(c, tempDir, []local.PackageInstance{inst})

		Convey("FetchInstance (no cache)", func() {
			reader, err := client.FetchInstance(ctx, inst.Pin())
			So(err, ShouldBeNil)

			// Backed by a temp file.
			tmpFile := reader.(deleteOnClose)
			_, err = os.Stat(tmpFile.Name())
			So(err, ShouldBeNil)

			fetched, err := local.OpenInstance(ctx, reader, "")
			So(err, ShouldBeNil)
			So(fetched.Pin(), ShouldResemble, inst.Pin())
			fetched.Close() // this closes the 'reader' too

			// The temp file is gone.
			_, err = os.Stat(tmpFile.Name())
			So(os.IsNotExist(err), ShouldBeTrue)
		})

		Convey("FetchInstance (with cache)", func() {
			client.CacheDir = filepath.Join(tempDir, "instance_cache")

			reader, err := client.FetchInstance(ctx, inst.Pin())
			So(err, ShouldBeNil)

			// Backed by a real file.
			cachedFile := reader.(*os.File)
			info1, err := os.Stat(cachedFile.Name())
			So(err, ShouldBeNil)

			fetched, err := local.OpenInstance(ctx, reader, "")
			So(err, ShouldBeNil)
			So(fetched.Pin(), ShouldResemble, inst.Pin())
			fetched.Close() // this closes the 'reader' too

			// The real file is still there, in the cache.
			_, err = os.Stat(cachedFile.Name())
			So(err, ShouldBeNil)

			// Fetch again.
			reader, err = client.FetchInstance(ctx, inst.Pin())
			So(err, ShouldBeNil)

			// Got same exact file.
			cachedFile = reader.(*os.File)
			info2, err := os.Stat(cachedFile.Name())
			So(err, ShouldBeNil)
			So(os.SameFile(info1, info2), ShouldBeTrue)

			reader.Close()
		})

		Convey("FetchInstanceTo (no cache)", func() {
			tempFile := filepath.Join(tempDir, "pkg")
			out, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_CREATE, 0666)
			So(err, ShouldBeNil)
			defer out.Close()

			err = client.FetchInstanceTo(ctx, inst.Pin(), out)
			So(err, ShouldBeNil)
			out.Close()

			fetched, err := local.OpenInstanceFile(ctx, tempFile, "")
			So(err, ShouldBeNil)
			So(fetched.Pin(), ShouldResemble, inst.Pin())
			fetched.Close()
		})

		Convey("FetchInstanceTo (with cache)", func() {
			client.CacheDir = filepath.Join(tempDir, "instance_cache")

			tempFile := filepath.Join(tempDir, "pkg")
			out, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_CREATE, 0666)
			So(err, ShouldBeNil)
			defer out.Close()

			err = client.FetchInstanceTo(ctx, inst.Pin(), out)
			So(err, ShouldBeNil)
			out.Close()

			fetched, err := local.OpenInstanceFile(ctx, tempFile, "")
			So(err, ShouldBeNil)
			So(fetched.Pin(), ShouldResemble, inst.Pin())
			fetched.Close()
		})

		Convey("FetchAndDeployInstance works", func() {
			// Install the package, fetching it from the fake server.
			err := client.FetchAndDeployInstance(ctx, "", inst.Pin())
			So(err, ShouldBeNil)

			// The file from the package should be installed.
			data, err := ioutil.ReadFile(filepath.Join(tempDir, "file"))
			So(err, ShouldBeNil)
			So(data, ShouldResemble, []byte("test data"))
		})
	})
}

func TestMaybeUpdateClient(t *testing.T) {
	ctx := makeTestContext()

	Convey("MaybeUpdateClient", t, func() {
		Convey("Is a NOOP when exeHash matches", func(c C) {
			client := mockClient(c, "", nil)
			client.tagCache = internal.NewTagCache(nil, "service.example.com")
			pin := Pin{clientPackage, "0000000000000000000000000000000000000000"}
			So(client.tagCache.AddTag(ctx, pin, "git:deadbeef"), ShouldBeNil)
			So(client.tagCache.AddFile(ctx, pin, clientFileName, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), ShouldBeNil)
			pin, err := client.maybeUpdateClient(ctx, nil, "git:deadbeef", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "some_path")
			So(pin, ShouldResemble, pin)
			So(err, ShouldBeNil)
		})
	})
}

func TestListPackages(t *testing.T) {
	ctx := makeTestContext()

	call := func(c C, dirPath string, recursive bool, calls []expectedHTTPCall) ([]string, error) {
		client := mockClient(c, "", calls)
		return client.ListPackages(ctx, dirPath, recursive, false)
	}

	Convey("ListPackages merges directories", t, func(c C) {
		out, err := call(c, "", true, []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/package/search",
				Query: url.Values{
					"path":        []string{""},
					"recursive":   []string{"true"},
					"show_hidden": []string{"false"},
				},
				Reply: `{"status":"SUCCESS","packages":["dir/pkg"],"directories":["dir"]}`,
			},
		})
		So(err, ShouldBeNil)
		So(out, ShouldResemble, []string{"dir/", "dir/pkg"})
	})
}

func TestEnsurePackages(t *testing.T) {
	ctx := makeTestContext()

	Convey("Mocking temp dir", t, func() {
		tempDir, err := ioutil.TempDir("", "cipd_test")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)

		assertFile := func(relPath, data string) {
			body, err := ioutil.ReadFile(filepath.Join(tempDir, relPath))
			So(err, ShouldBeNil)
			So(string(body), ShouldEqual, data)
		}

		Convey("EnsurePackages full flow", func(c C) {
			// Prepare a bunch of packages.
			a1 := buildInstanceInMemory(ctx, "pkg/a", []local.File{local.NewTestFile("file a 1", "test data", false)})
			defer a1.Close()
			a2 := buildInstanceInMemory(ctx, "pkg/a", []local.File{local.NewTestFile("file a 2", "test data", false)})
			defer a2.Close()
			b := buildInstanceInMemory(ctx, "pkg/b", []local.File{local.NewTestFile("file b", "test data", false)})
			defer b.Close()

			// Calls EnsurePackages, mocking fetch backend first. Backend will be mocked
			// to serve only 'fetched' packages.
			callEnsure := func(instances []local.PackageInstance, fetched []local.PackageInstance) (Actions, error) {
				client := mockClientForFetch(c, tempDir, fetched)
				pins := PinSliceBySubdir{}
				for _, i := range instances {
					pins[""] = append(pins[""], i.Pin())
				}
				return client.EnsurePackages(ctx, pins, false)
			}

			findDeployed := func(root string) PinSliceBySubdir {
				deployer := local.NewDeployer(root)
				pins, err := deployer.FindDeployed(ctx)
				So(err, ShouldBeNil)
				return pins
			}

			// Noop run on top of empty directory.
			actions, err := callEnsure(nil, nil)
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, Actions{})

			// Specify same package twice. Fails.
			actions, err = callEnsure([]local.PackageInstance{a1, a2}, nil)
			So(err, ShouldNotBeNil)
			So(actions, ShouldResemble, Actions{})

			// Install a1 into a site root.
			actions, err = callEnsure([]local.PackageInstance{a1}, []local.PackageInstance{a1})
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, Actions{
				ToInstall: PinSlice{a1.Pin()},
			})
			assertFile("file a 1", "test data")
			So(findDeployed(tempDir), ShouldResemble, PinSliceBySubdir{"": PinSlice{a1.Pin()}})

			// Noop run. Nothing is fetched.
			actions, err = callEnsure([]local.PackageInstance{a1}, nil)
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, Actions{})
			assertFile("file a 1", "test data")
			So(findDeployed(tempDir), ShouldResemble, PinSliceBySubdir{"": PinSlice{a1.Pin()}})

			// Upgrade a1 to a2.
			actions, err = callEnsure([]local.PackageInstance{a2}, []local.PackageInstance{a2})
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, Actions{
				ToUpdate: []UpdatedPin{
					{
						From: a1.Pin(),
						To:   a2.Pin(),
					},
				},
			})
			assertFile("file a 2", "test data")
			So(findDeployed(tempDir), ShouldResemble, PinSliceBySubdir{"": PinSlice{a2.Pin()}})

			// Remove a2 and install b.
			actions, err = callEnsure([]local.PackageInstance{b}, []local.PackageInstance{b})
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, Actions{
				ToInstall: PinSlice{b.Pin()},
				ToRemove:  PinSlice{a2.Pin()},
			})
			assertFile("file b", "test data")
			So(findDeployed(tempDir), ShouldResemble, PinSliceBySubdir{"": PinSlice{b.Pin()}})

			// Remove b.
			actions, err = callEnsure(nil, nil)
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, Actions{
				ToRemove: PinSlice{b.Pin()},
			})
			So(findDeployed(tempDir), ShouldResemble, PinSliceBySubdir{"": PinSlice{}})

			// Install a1 and b.
			actions, err = callEnsure([]local.PackageInstance{a1, b}, []local.PackageInstance{a1, b})
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, Actions{
				ToInstall: PinSlice{a1.Pin(), b.Pin()},
			})
			assertFile("file a 1", "test data")
			assertFile("file b", "test data")
			So(findDeployed(tempDir), ShouldResemble, PinSliceBySubdir{"": PinSlice{a1.Pin(), b.Pin()}})
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

// buildInstanceInMemory makes fully functional PackageInstance object that uses
// memory buffer as a backing store.
func buildInstanceInMemory(ctx context.Context, pkgName string, files []local.File) local.PackageInstance {
	out := bytes.Buffer{}
	err := local.BuildInstance(ctx, local.BuildInstanceOptions{
		Input:            files,
		Output:           &out,
		PackageName:      pkgName,
		CompressionLevel: 5,
	})
	So(err, ShouldBeNil)
	inst, err := local.OpenInstance(ctx, bytes.NewReader(out.Bytes()), "")
	So(err, ShouldBeNil)
	return inst
}

////////////////////////////////////////////////////////////////////////////////

// mockClientForFetch returns Client with fetch related calls mocked.
func mockClientForFetch(c C, root string, instances []local.PackageInstance) *clientImpl {
	// Mock RPC calls.
	calls := []expectedHTTPCall{}
	for _, inst := range instances {
		calls = append(calls, expectedHTTPCall{
			Method: "GET",
			Path:   "/_ah/api/repo/v1/instance",
			Query: url.Values{
				"instance_id":  []string{inst.Pin().InstanceID},
				"package_name": []string{inst.Pin().PackageName},
			},
			Reply: fmt.Sprintf(`{
				"status": "SUCCESS",
				"instance": {
					"registered_by": "user:a@example.com",
					"registered_ts": "0"
				},
				"fetch_url": "http://localhost/fetch/%s"
			}`, inst.Pin().InstanceID),
		})
	}
	client := mockClient(c, root, calls)

	// Mock storage.
	data := map[string][]byte{}
	for _, inst := range instances {
		r := inst.DataReader()
		_, err := r.Seek(0, os.SEEK_SET)
		c.So(err, ShouldBeNil)
		blob, err := ioutil.ReadAll(r)
		c.So(err, ShouldBeNil)
		data["http://localhost/fetch/"+inst.Pin().InstanceID] = blob
	}
	client.storage = &mockedStorage{c, data}
	return client
}

////////////////////////////////////////////////////////////////////////////////

// mockedStorage implements storage by returning mocked data in 'download' and
// doing nothing in 'upload'.
type mockedStorage struct {
	c    C
	data map[string][]byte
}

func (s *mockedStorage) download(ctx context.Context, url string, output io.WriteSeeker) error {
	blob, ok := s.data[url]
	if !ok {
		return ErrDownloadError
	}
	_, err := output.Seek(0, os.SEEK_SET)
	s.c.So(err, ShouldBeNil)
	_, err = output.Write(blob)
	s.c.So(err, ShouldBeNil)
	return nil
}

func (s *mockedStorage) upload(ctx context.Context, url string, data io.ReadSeeker) error {
	return nil
}

////////////////////////////////////////////////////////////////////////////////

type expectedHTTPCall struct {
	Method          string
	Path            string
	Query           url.Values
	Body            string
	Headers         http.Header
	Reply           string
	Status          int
	ResponseHeaders http.Header
}

func makeTestContext() context.Context {
	ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
	tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
		tc.Add(d)
	})
	return gologger.StdConfig.Use(ctx)
}

// mockClient returns Client with clock and HTTP calls mocked.
func mockClient(c C, root string, expectations []expectedHTTPCall) *clientImpl {
	// Provide fake client instead.
	handler := &expectedHTTPCallHandler{c, expectations, 0}
	server := httptest.NewServer(handler)
	Reset(func() {
		server.Close()
		// All expected calls should be made.
		if handler.index != len(handler.calls) {
			c.Printf("Unfinished calls: %v\n", handler.calls[handler.index:])
		}
		c.So(handler.index, ShouldEqual, len(handler.calls))
	})
	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	client, err := NewClient(ClientOptions{
		ServiceURL:          server.URL,
		Root:                root,
		AnonymousClient:     &http.Client{Transport: transport},
		AuthenticatedClient: &http.Client{Transport: transport},
	})
	c.So(err, ShouldBeNil)
	return client.(*clientImpl)
}

// expectedHTTPCallHandler is http.Handler that serves mocked HTTP calls.
type expectedHTTPCallHandler struct {
	c     C
	calls []expectedHTTPCall
	index int
}

func (s *expectedHTTPCallHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Unexpected call?
	if s.index == len(s.calls) {
		s.c.Printf("Unexpected call: %v\n", r)
	}
	s.c.So(s.index, ShouldBeLessThan, len(s.calls))

	// Fill in defaults.
	exp := s.calls[s.index]
	if exp.Method == "" {
		exp.Method = "GET"
	}
	if exp.Query == nil {
		exp.Query = url.Values{}
	}
	if exp.Headers == nil {
		exp.Headers = http.Header{}
	}

	// Read body and essential headers.
	body, err := ioutil.ReadAll(r.Body)
	s.c.So(err, ShouldBeNil)
	blacklist := map[string]bool{
		"Accept-Encoding": true,
		"Content-Length":  true,
		"Content-Type":    true,
		"User-Agent":      true,
	}
	headers := http.Header{}
	for k, v := range r.Header {
		_, isExpected := exp.Headers[k]
		if isExpected || !blacklist[k] {
			headers[k] = v
		}
	}

	// Check that request is what it is expected to be.
	s.c.So(r.Method, ShouldEqual, exp.Method)
	s.c.So(r.URL.Path, ShouldEqual, exp.Path)
	s.c.So(r.URL.Query(), ShouldResemble, exp.Query)
	s.c.So(headers, ShouldResemble, exp.Headers)
	s.c.So(string(body), ShouldEqual, exp.Body)

	// Mocked reply.
	if exp.Status != 0 {
		for k, v := range exp.ResponseHeaders {
			for _, s := range v {
				w.Header().Add(k, s)
			}
		}
		w.WriteHeader(exp.Status)
	}
	if exp.Reply != "" {
		w.Write([]byte(exp.Reply))
	}
	s.index++
}
