// Copyright 2015 The LUCI Authors.
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
	"bytes"
	"fmt"
	"hash"
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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/gologger"

	. "go.chromium.org/luci/cipd/client/cipd/common"
	"go.chromium.org/luci/cipd/client/cipd/internal"
	"go.chromium.org/luci/cipd/client/cipd/local"

	. "github.com/smartystreets/goconvey/convey"
)

func underlyingFile(i local.InstanceFile) *os.File {
	return i.(interface {
		UnderlyingFile() *os.File
	}).UnderlyingFile()
}

type bytesInstanceFile struct {
	*bytes.Reader
}

func (bytesInstanceFile) Close(context.Context, bool) error { return nil }

func bytesFile(data []byte) local.InstanceFile {
	return bytesInstanceFile{bytes.NewReader(data)}
}

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
		inst, err := local.OpenInstance(ctx, bytesFile(out.Bytes()), "", local.VerifyHash)
		So(err, ShouldBeNil)

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

		Convey("fetching once", func() {
			client := mockClientForFetch(c, tempDir, []local.PackageInstance{inst})

			Convey("FetchInstance (no cache)", func() {
				reader, err := client.FetchInstance(ctx, inst.Pin())
				So(err, ShouldBeNil)
				defer reader.Close(ctx, false) // just in case

				// Backed by a temp file.
				tmpFile := reader.(deleteOnClose)
				_, err = os.Stat(tmpFile.Name())
				So(err, ShouldBeNil)

				fetched, err := local.OpenInstance(ctx, reader, "", local.VerifyHash)
				So(err, ShouldBeNil)
				So(fetched.Pin(), ShouldResemble, inst.Pin())
				tmpFile.Close(ctx, false)

				// The temp file is gone.
				_, err = os.Stat(tmpFile.Name())
				So(os.IsNotExist(err), ShouldBeTrue)
			})

			Convey("FetchInstance (with cache)", func() {
				client.CacheDir = filepath.Join(tempDir, "instance_cache")

				reader, err := client.FetchInstance(ctx, inst.Pin())
				So(err, ShouldBeNil)
				defer reader.Close(ctx, false)

				// Backed by a real file.
				cachedFile := underlyingFile(reader)
				info1, err := os.Stat(cachedFile.Name())
				So(err, ShouldBeNil)

				fetched, err := local.OpenInstance(ctx, reader, "", local.VerifyHash)
				So(err, ShouldBeNil)
				So(fetched.Pin(), ShouldResemble, inst.Pin())

				// The real file is still there, in the cache.
				_, err = os.Stat(cachedFile.Name())
				So(err, ShouldBeNil)

				// Fetch again.
				reader, err = client.FetchInstance(ctx, inst.Pin())
				So(err, ShouldBeNil)

				// Got same exact file.
				cachedFile = underlyingFile(reader)
				info2, err := os.Stat(cachedFile.Name())
				So(err, ShouldBeNil)
				So(os.SameFile(info1, info2), ShouldBeTrue)

				reader.Close(ctx, false)
			})

			Convey("FetchInstanceTo (no cache)", func() {
				tempFile := filepath.Join(tempDir, "pkg")
				out, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_CREATE, 0666)
				So(err, ShouldBeNil)
				defer out.Close()

				err = client.FetchInstanceTo(ctx, inst.Pin(), out)
				So(err, ShouldBeNil)
				out.Close()

				fetched, closer, err := local.OpenInstanceFile(ctx, tempFile, "", local.VerifyHash)
				So(err, ShouldBeNil)
				defer closer()
				So(fetched.Pin(), ShouldResemble, inst.Pin())
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

				fetched, closer, err := local.OpenInstanceFile(ctx, tempFile, "", local.VerifyHash)
				So(err, ShouldBeNil)
				defer closer()
				So(fetched.Pin(), ShouldResemble, inst.Pin())
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

		Convey("FetchAndDeployInstance works with busted cache", func() {
			client := mockClientForFetch(c, tempDir, []local.PackageInstance{inst, inst})
			client.CacheDir = filepath.Join(tempDir, "instance_cache")
			cache := client.getInstanceCache(ctx)

			// Install the package, fetching it from the fake server.
			err := client.FetchAndDeployInstance(ctx, "", inst.Pin())
			So(err, ShouldBeNil)

			// The file from the package should be installed.
			data, err := ioutil.ReadFile(filepath.Join(tempDir, "file"))
			So(err, ShouldBeNil)
			So(data, ShouldResemble, []byte("test data"))

			// we can now fetch the cached file
			cachedFile, err := cache.Get(ctx, inst.Pin(), clock.Now(ctx))
			So(err, ShouldBeNil)
			So(cachedFile.Close(ctx, false), ShouldBeNil)

			// now we goof up the cached file
			err = ioutil.WriteFile(underlyingFile(cachedFile).Name(), []byte("bananas"), 0666)
			So(err, ShouldBeNil)

			// uninstall the package
			err = client.deployer.RemoveDeployed(ctx, "", inst.Pin().PackageName)
			So(err, ShouldBeNil)

			// Install the package again. This time we should hit, and invalidate,
			// the cache.
			err = client.FetchAndDeployInstance(ctx, "", inst.Pin())
			So(err, ShouldBeNil)

			// the cache file is different
			cachedFile, err = cache.Get(ctx, inst.Pin(), clock.Now(ctx))
			So(err, ShouldBeNil)
			So(cachedFile.Close(ctx, false), ShouldBeNil)

			data, err = ioutil.ReadFile(underlyingFile(cachedFile).Name())
			So(err, ShouldBeNil)
			So(string(data), ShouldNotStartWith, "bananas")

			// However, the file from the package should be installed again.
			data, err = ioutil.ReadFile(filepath.Join(tempDir, "file"))
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

		shouldHaveContent := func(relPath interface{}, data ...interface{}) string {
			body, err := ioutil.ReadFile(filepath.Join(tempDir, relPath.(string)))
			if ret := ShouldBeNil(err); ret != "" {
				return ret
			}
			return ShouldEqual(string(body), data[0].(string))
		}

		Convey("EnsurePackages full flow", func(c C) {
			// Prepare a bunch of packages.
			a1 := buildInstanceInMemory(ctx, "pkg/a", []local.File{local.NewTestFile("file a 1", "test data", false)})
			a2 := buildInstanceInMemory(ctx, "pkg/a", []local.File{local.NewTestFile("file a 2", "test data", false)})
			b := buildInstanceInMemory(ctx, "pkg/b", []local.File{local.NewTestFile("file b", "test data", false)})

			pil := func(insts ...local.PackageInstance) []local.PackageInstance {
				return insts
			}

			// Calls EnsurePackages, mocking fetch backend first. Backend will be mocked
			// to serve only 'fetched' packages. callEnsure will ensure the state
			// reflected by the 'state' variable.
			state := map[string][]local.PackageInstance{}
			callEnsure := func(fetched ...local.PackageInstance) (ActionMap, error) {
				client := mockClientForFetch(c, tempDir, fetched)
				pins := PinSliceBySubdir{}
				for subdir, instances := range state {
					for _, i := range instances {
						pins[subdir] = append(pins[subdir], i.Pin())
					}
				}
				return client.EnsurePackages(ctx, pins, false)
			}

			shouldBeDeployed := func(expect interface{}, _ ...interface{}) string {
				deployer := local.NewDeployer(tempDir)
				pins, err := deployer.FindDeployed(ctx)
				if ret := ShouldBeNil(err); ret != "" {
					return ret
				}
				return ShouldResemble(pins, expect.(PinSliceBySubdir))
			}

			// Noop run on top of empty directory.
			actions, err := callEnsure()
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, ActionMap(nil))

			// Specify same package twice. Fails.
			state[""] = pil(a1, a2)
			actions, err = callEnsure()
			So(err, ShouldNotBeNil)
			So(actions, ShouldResemble, ActionMap(nil))

			// Install a1 into a site root.
			state[""] = pil(a1)
			actions, err = callEnsure(a1)
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, ActionMap{
				"": &Actions{ToInstall: PinSlice{a1.Pin()}},
			})
			So("file a 1", shouldHaveContent, "test data")
			So(PinSliceBySubdir{
				"": PinSlice{a1.Pin()},
			}, shouldBeDeployed)

			// Install a1 into subdir, remove it from root.
			state[""] = nil
			state["subdir"] = pil(a1)
			actions, err = callEnsure(a1)
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, ActionMap{
				"":       &Actions{ToRemove: PinSlice{a1.Pin()}},
				"subdir": &Actions{ToInstall: PinSlice{a1.Pin()}},
			})
			So("subdir/file a 1", shouldHaveContent, "test data")
			So(PinSliceBySubdir{
				"subdir": PinSlice{a1.Pin()},
			}, shouldBeDeployed)

			// Noop run. Nothing is fetched.
			actions, err = callEnsure()
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, ActionMap(nil))
			So("subdir/file a 1", shouldHaveContent, "test data")
			So(PinSliceBySubdir{
				"subdir": PinSlice{a1.Pin()},
			}, shouldBeDeployed)

			// Root and subdir installed at the same time.
			state[""] = pil(a1)
			actions, err = callEnsure(a1)
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, ActionMap{
				"": &Actions{ToInstall: PinSlice{a1.Pin()}},
			})
			So("subdir/file a 1", shouldHaveContent, "test data")
			So("file a 1", shouldHaveContent, "test data")
			So(PinSliceBySubdir{
				"":       PinSlice{a1.Pin()},
				"subdir": PinSlice{a1.Pin()},
			}, shouldBeDeployed)

			// Upgrade a1 to a2.
			state[""] = pil(a2)
			actions, err = callEnsure(a2)
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, ActionMap{
				"": &Actions{ToUpdate: []UpdatedPin{
					{
						From: a1.Pin(),
						To:   a2.Pin(),
					},
				}},
			})
			So("file a 2", shouldHaveContent, "test data")
			So(PinSliceBySubdir{
				"":       PinSlice{a2.Pin()},
				"subdir": PinSlice{a1.Pin()},
			}, shouldBeDeployed)

			// Remove a2 and install b.
			state[""] = pil(b)
			actions, err = callEnsure(b)
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, ActionMap{
				"": &Actions{
					ToInstall: PinSlice{b.Pin()},
					ToRemove:  PinSlice{a2.Pin()},
				},
			})
			So("file b", shouldHaveContent, "test data")
			So(PinSliceBySubdir{
				"":       PinSlice{b.Pin()},
				"subdir": PinSlice{a1.Pin()},
			}, shouldBeDeployed)

			// Remove b.
			state[""] = nil
			actions, err = callEnsure()
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, ActionMap{
				"": &Actions{
					ToRemove: PinSlice{b.Pin()},
				},
			})
			So(PinSliceBySubdir{
				"subdir": PinSlice{a1.Pin()},
			}, shouldBeDeployed)

			// Remove a1 from subdir
			state["subdir"] = nil
			actions, err = callEnsure()
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, ActionMap{
				"subdir": &Actions{
					ToRemove: PinSlice{a1.Pin()},
				},
			})
			So(PinSliceBySubdir{}, shouldBeDeployed)

			// Install a1 and b.
			state[""] = pil(a1, b)
			actions, err = callEnsure(a1, b)
			So(err, ShouldBeNil)
			So(actions, ShouldResemble, ActionMap{
				"": &Actions{
					ToInstall: PinSlice{a1.Pin(), b.Pin()},
				},
			})
			So("file a 1", shouldHaveContent, "test data")
			So("file b", shouldHaveContent, "test data")
			So(PinSliceBySubdir{
				"": PinSlice{a1.Pin(), b.Pin()},
			}, shouldBeDeployed)
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
	inst, err := local.OpenInstance(ctx, bytesFile(out.Bytes()), "", local.VerifyHash)
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

func (s *mockedStorage) download(ctx context.Context, url string, output io.WriteSeeker, h hash.Hash) error {
	blob, ok := s.data[url]
	if !ok {
		return ErrDownloadError
	}
	h.Reset()
	_, err := output.Seek(0, os.SEEK_SET)
	s.c.So(err, ShouldBeNil)
	_, err = output.Write(blob)
	s.c.So(err, ShouldBeNil)
	_, err = h.Write(blob)
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
