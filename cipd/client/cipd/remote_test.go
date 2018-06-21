// Copyright 2014 The LUCI Authors.
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
	"net/url"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/cipd/common"
)

func TestRemoteImpl(t *testing.T) {
	ctx := makeTestContext()

	mockInitiateUpload := func(c C, reply string) (*UploadSession, error) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/upload/SHA1/abc",
				Reply:  reply,
			},
		})
		return remote.initiateUpload(ctx, "abc")
	}

	mockFinalizeUpload := func(c C, reply string) (bool, error) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/finalize/abc",
				Reply:  reply,
			},
		})
		return remote.finalizeUpload(ctx, "abc")
	}

	mockRegisterInstance := func(c C, reply string) (*registerInstanceResponse, error) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/repo/v1/instance",
				Query: url.Values{
					"package_name": []string{"pkgname"},
					"instance_id":  []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				},
				Reply: reply,
			},
		})
		return remote.registerInstance(ctx, Pin{"pkgname", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	}

	mockFetchPackageRefs := func(c C, reply string) ([]RefInfo, error) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/package",
				Query: url.Values{
					"package_name": []string{"pkgname"},
					"with_refs":    []string{"true"},
				},
				Reply: reply,
			},
		})
		return remote.fetchPackageRefs(ctx, "pkgname")
	}

	mockFetchInstanceImpl := func(c C, reply string) (*fetchInstanceResponse, string, error) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/instance",
				Query: url.Values{
					"package_name": []string{"pkgname"},
					"instance_id":  []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				},
				Reply: reply,
			},
		})
		return remote.fetchInstanceImpl(ctx, Pin{"pkgname", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	}

	mockFetchClientBinaryInfo := func(c C, reply string) (*fetchClientBinaryInfoResponse, error) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/client",
				Query: url.Values{
					"package_name": []string{"infra/tools/cipd/mac-amd64"},
					"instance_id":  []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				},
				Reply: reply,
			},
		})
		return remote.fetchClientBinaryInfo(ctx,
			Pin{"infra/tools/cipd/mac-amd64", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	}

	mockFetchTags := func(c C, reply string, tags []string) ([]TagInfo, error) {
		query := url.Values{
			"package_name": []string{"pkgname"},
			"instance_id":  []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		}
		if len(tags) != 0 {
			query["tags"] = tags
		}
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/tags",
				Query:  query,
				Reply:  reply,
			},
		})
		return remote.fetchTags(ctx, Pin{"pkgname", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, tags)
	}

	mockFetchRefs := func(c C, reply string, refs []string) ([]RefInfo, error) {
		query := url.Values{
			"package_name": []string{"pkgname"},
			"instance_id":  []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		}
		if len(refs) != 0 {
			query["refs"] = refs
		}
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/ref",
				Query:  query,
				Reply:  reply,
			},
		})
		return remote.fetchRefs(ctx, Pin{"pkgname", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, refs)
	}

	mockFetchACL := func(c C, reply string) ([]PackageACL, error) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/acl",
				Query:  url.Values{"package_path": []string{"pkgname"}},
				Reply:  reply,
			},
		})
		return remote.fetchACL(ctx, "pkgname")
	}

	mockModifyACL := func(c C, changes []PackageACLChange, body, reply string) error {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/repo/v1/acl",
				Query:  url.Values{"package_path": []string{"pkgname"}},
				Body:   body,
				Reply:  reply,
			},
		})
		return remote.modifyACL(ctx, "pkgname", changes)
	}

	mockSetRef := func(c C, reply string) error {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/repo/v1/ref",
				Query: url.Values{
					"package_name": []string{"pkgname"},
					"ref":          []string{"some-ref"},
				},
				Body:  `{"instance_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`,
				Reply: reply,
			},
		})
		return remote.setRef(ctx, "some-ref", Pin{"pkgname", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	}

	mockListPackages := func(c C, reply string) ([]string, []string, error) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/package/search",
				Query: url.Values{
					"path":        []string{"pkgpath"},
					"recursive":   []string{"false"},
					"show_hidden": []string{"false"},
				},
				Reply: reply,
			},
		})
		return remote.listPackages(ctx, "pkgpath", false, false)
	}

	mockSearchInstances := func(c C, reply string) (PinSlice, error) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/instance/search",
				Query: url.Values{
					"package_name": []string{"pkgname"},
					"tag":          []string{"tag:v"},
				},
				Reply: reply,
			},
		})
		return remote.searchInstances(ctx, "tag:v", "pkgname")
	}

	mockListInstances := func(c C, reply string) (*listInstancesResponse, error) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/instances",
				Query: url.Values{
					"package_name": []string{"pkgname"},
					"limit":        []string{"123"},
					"cursor":       []string{"cursor_value"},
				},
				Reply: reply,
			},
		})
		return remote.listInstances(ctx, "pkgname", 123, "cursor_value")
	}

	mockAttachTags := func(c C, tags []string, body, reply string) error {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/repo/v1/tags",
				Query: url.Values{
					"package_name": []string{"pkgname"},
					"instance_id":  []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				},
				Body:  body,
				Reply: reply,
			},
		})
		return remote.attachTags(ctx, Pin{"pkgname", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, tags)
	}

	mockResolveVersion := func(c C, reply string) (Pin, error) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/repo/v1/instance/resolve",
				Query: url.Values{
					"package_name": []string{"pkgname"},
					"version":      []string{"tag_key:value"},
				},
				Reply: reply,
			},
		})
		return remote.resolveVersion(ctx, "pkgname", "tag_key:value")
	}

	Convey("makeRequest POST works", t, func(c C) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/method",
				Reply:  `{"value":"123"}`,
			},
		})
		var reply struct {
			Value string `json:"value"`
		}
		err := remote.makeRequest(ctx, "cas/v1/method", "POST", nil, &reply)
		So(err, ShouldBeNil)
		So(reply.Value, ShouldEqual, "123")
	})

	Convey("makeRequest GET works", t, func(c C) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "GET",
				Path:   "/_ah/api/cas/v1/method",
				Reply:  `{"value":"123"}`,
			},
		})
		var reply struct {
			Value string `json:"value"`
		}
		err := remote.makeRequest(ctx, "cas/v1/method", "GET", nil, &reply)
		So(err, ShouldBeNil)
		So(reply.Value, ShouldEqual, "123")
	})

	Convey("makeRequest handles fatal error", t, func(c C) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/method",
				Status: 403,
			},
		})
		var reply struct{}
		err := remote.makeRequest(ctx, "cas/v1/method", "POST", nil, &reply)
		So(err, ShouldNotBeNil)
	})

	Convey("makeRequest handles retries", t, func(c C) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/method",
				Status: 500,
			},
			{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/method",
				Reply:  `{}`,
			},
		})
		var reply struct{}
		err := remote.makeRequest(ctx, "cas/v1/method", "POST", nil, &reply)
		So(err, ShouldBeNil)
	})

	Convey("makeRequest gives up trying", t, func(c C) {
		calls := []expectedHTTPCall{}
		for i := 0; i < remoteMaxRetries; i++ {
			calls = append(calls, expectedHTTPCall{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/method",
				Status: 500,
			})
		}
		remote := mockRemoteImpl(c, calls)
		var reply struct{}
		err := remote.makeRequest(ctx, "cas/v1/method", "POST", nil, &reply)
		So(err, ShouldNotBeNil)
	})

	Convey("initiateUpload ALREADY_UPLOADED", t, func(c C) {
		s, err := mockInitiateUpload(c, `{"status":"ALREADY_UPLOADED"}`)
		So(err, ShouldBeNil)
		So(s, ShouldBeNil)
	})

	Convey("initiateUpload SUCCESS", t, func(c C) {
		s, err := mockInitiateUpload(c, `{"status":"SUCCESS","upload_session_id":"123","upload_url":"http://localhost"}`)
		So(err, ShouldBeNil)
		So(s, ShouldResemble, &UploadSession{"123", "http://localhost"})
	})

	Convey("initiateUpload ERROR", t, func(c C) {
		s, err := mockInitiateUpload(c, `{"status":"ERROR","error_message":"boo"}`)
		So(err, ShouldNotBeNil)
		So(s, ShouldBeNil)
	})

	Convey("initiateUpload unknown status", t, func(c C) {
		s, err := mockInitiateUpload(c, `{"status":"???"}`)
		So(err, ShouldNotBeNil)
		So(s, ShouldBeNil)
	})

	Convey("initiateUpload bad reply", t, func(c C) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/upload/SHA1/abc",
				Status: 403,
			},
		})
		s, err := remote.initiateUpload(ctx, "abc")
		So(err, ShouldNotBeNil)
		So(s, ShouldBeNil)
	})

	Convey("finalizeUpload MISSING", t, func(c C) {
		finished, err := mockFinalizeUpload(c, `{"status":"MISSING"}`)
		So(err, ShouldNotBeNil)
		So(finished, ShouldBeFalse)
	})

	Convey("finalizeUpload UPLOADING", t, func(c C) {
		finished, err := mockFinalizeUpload(c, `{"status":"UPLOADING"}`)
		So(err, ShouldBeNil)
		So(finished, ShouldBeFalse)
	})

	Convey("finalizeUpload VERIFYING", t, func(c C) {
		finished, err := mockFinalizeUpload(c, `{"status":"VERIFYING"}`)
		So(err, ShouldBeNil)
		So(finished, ShouldBeFalse)
	})

	Convey("finalizeUpload PUBLISHED", t, func(c C) {
		finished, err := mockFinalizeUpload(c, `{"status":"PUBLISHED"}`)
		So(err, ShouldBeNil)
		So(finished, ShouldBeTrue)
	})

	Convey("finalizeUpload ERROR", t, func(c C) {
		finished, err := mockFinalizeUpload(c, `{"status":"ERROR","error_message":"boo"}`)
		So(err, ShouldNotBeNil)
		So(finished, ShouldBeFalse)
	})

	Convey("finalizeUpload unknown status", t, func(c C) {
		finished, err := mockFinalizeUpload(c, `{"status":"???"}`)
		So(err, ShouldNotBeNil)
		So(finished, ShouldBeFalse)
	})

	Convey("finalizeUpload bad reply", t, func(c C) {
		remote := mockRemoteImpl(c, []expectedHTTPCall{
			{
				Method: "POST",
				Path:   "/_ah/api/cas/v1/finalize/abc",
				Status: 403,
			},
		})
		finished, err := remote.finalizeUpload(ctx, "abc")
		So(err, ShouldNotBeNil)
		So(finished, ShouldBeFalse)
	})

	Convey("registerInstance REGISTERED", t, func(c C) {
		result, err := mockRegisterInstance(c, `{
				"status": "REGISTERED",
				"instance": {
					"registered_by": "user:abc@example.com",
					"registered_ts": "1420244414571500"
				}
			}`)
		So(err, ShouldBeNil)
		So(result, ShouldResemble, &registerInstanceResponse{
			registeredBy: "user:abc@example.com",
			registeredTs: time.Unix(0, 1420244414571500000),
		})
	})

	Convey("registerInstance ALREADY_REGISTERED", t, func(c C) {
		result, err := mockRegisterInstance(c, `{
				"status": "ALREADY_REGISTERED",
				"instance": {
					"registered_by": "user:abc@example.com",
					"registered_ts": "1420244414571500"
				}
			}`)
		So(err, ShouldBeNil)
		So(result, ShouldResemble, &registerInstanceResponse{
			alreadyRegistered: true,
			registeredBy:      "user:abc@example.com",
			registeredTs:      time.Unix(0, 1420244414571500000),
		})
	})

	Convey("registerInstance UPLOAD_FIRST", t, func(c C) {
		result, err := mockRegisterInstance(c, `{
				"status": "UPLOAD_FIRST",
				"upload_session_id": "upload_session_id",
				"upload_url": "http://upload_url"
			}`)
		So(err, ShouldBeNil)
		So(result, ShouldResemble, &registerInstanceResponse{
			uploadSession: &UploadSession{"upload_session_id", "http://upload_url"},
		})
	})

	Convey("registerInstance ERROR", t, func(c C) {
		result, err := mockRegisterInstance(c, `{
				"status": "ERROR",
				"error_message": "Some error message"
			}`)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
	})

	Convey("registerInstance unknown status", t, func(c C) {
		result, err := mockRegisterInstance(c, `{"status":"???"}`)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
	})

	Convey("fetchPackageRefs SUCCESS", t, func(c C) {
		refs, err := mockFetchPackageRefs(c, `{
				"status": "SUCCESS",
				"refs": [
					{
						"ref": "ref1",
						"modified_by": "user:a@example.com",
						"modified_ts": "1420244414571500"
					},
					{
						"ref": "ref2",
						"modified_by": "user:a@example.com",
						"modified_ts": "1420244414571500"
					}
				]
			}`)
		So(err, ShouldBeNil)
		So(refs, ShouldResemble, []RefInfo{
			{
				Ref:        "ref1",
				ModifiedBy: "user:a@example.com",
				ModifiedTs: UnixTime(time.Unix(0, 1420244414571500000)),
			},
			{
				Ref:        "ref2",
				ModifiedBy: "user:a@example.com",
				ModifiedTs: UnixTime(time.Unix(0, 1420244414571500000)),
			},
		})
	})

	Convey("fetchPackageRefs PACKAGE_NOT_FOUND", t, func(c C) {
		_, err := mockFetchPackageRefs(c, `{"status": "PACKAGE_NOT_FOUND"}`)
		So(err, ShouldNotBeNil)
	})

	Convey("fetchPackageRefs ERROR", t, func(c C) {
		_, err := mockFetchPackageRefs(c, `{
				"status": "ERROR",
				"error_message": "Some error message"
			}`)
		So(err, ShouldNotBeNil)
	})

	Convey("fetchInstanceImpl SUCCESS", t, func(c C) {
		result, url, err := mockFetchInstanceImpl(c, `{
				"status": "SUCCESS",
				"instance": {
					"registered_by": "user:abc@example.com",
					"registered_ts": "1420244414571500"
				},
				"fetch_url": "https://fetch_url"
			}`)
		So(err, ShouldBeNil)
		So(result, ShouldResemble, &fetchInstanceResponse{
			registeredBy: "user:abc@example.com",
			registeredTs: time.Unix(0, 1420244414571500000),
		})
		So(url, ShouldEqual, "https://fetch_url")
	})

	Convey("fetchInstanceImpl PACKAGE_NOT_FOUND", t, func(c C) {
		result, url, err := mockFetchInstanceImpl(c, `{"status": "PACKAGE_NOT_FOUND"}`)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
		So(url, ShouldEqual, "")
	})

	Convey("fetchInstanceImpl INSTANCE_NOT_FOUND", t, func(c C) {
		result, url, err := mockFetchInstanceImpl(c, `{"status": "INSTANCE_NOT_FOUND"}`)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
		So(url, ShouldEqual, "")
	})

	Convey("fetchInstanceImpl ERROR", t, func(c C) {
		result, url, err := mockFetchInstanceImpl(c, `{
				"status": "ERROR",
				"error_message": "Some error message"
			}`)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
		So(url, ShouldEqual, "")
	})

	Convey("fetchClientBinaryInfo SUCCESS", t, func(c C) {
		result, err := mockFetchClientBinaryInfo(c, `{
				"status": "SUCCESS",
				"instance": {
					"instance_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					"registered_by": "user:a@example.com",
					"registered_ts": "1420244414571500",
					"package_name": "infra/tools/cipd/mac-amd64"
				},
				"client_binary": {
					"file_name": "cipd",
					"sha1": "c52a8ffe7522302e5658fd7f1eb9e2dfd93c0b80",
					"fetch_url": "https://storage.googleapis.com/example?params=yes",
					"size": "10120188"
				},
				"kind": "repo#resourcesItem",
				"etag": "\"klmD7St9j1caKhEua58kkAgnm0A/GCbN4lyfRwANQtP3NW9-SWMggwY\""
			}`)
		So(err, ShouldBeNil)
		So(result, ShouldResemble, &fetchClientBinaryInfoResponse{
			instance: &InstanceInfo{
				Pin{"infra/tools/cipd/mac-amd64", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				"user:a@example.com",
				UnixTime(time.Unix(0, 1420244414571500000)),
			},
			clientBinary: &clientBinary{
				"cipd",
				"c52a8ffe7522302e5658fd7f1eb9e2dfd93c0b80",
				"https://storage.googleapis.com/example?params=yes",
				10120188,
			},
		})
	})

	Convey("fetchClientBinaryInfo PACKAGE_NOT_FOUND", t, func(c C) {
		result, err := mockFetchClientBinaryInfo(c, `{"status": "PACKAGE_NOT_FOUND"}`)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
	})

	Convey("fetchClientBinaryInfo INSTANCE_NOT_FOUND", t, func(c C) {
		result, err := mockFetchClientBinaryInfo(c, `{"status": "INSTANCE_NOT_FOUND"}`)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
	})

	Convey("fetchClientBinaryInfo ERROR", t, func(c C) {
		result, err := mockFetchClientBinaryInfo(c, `{
				"status": "ERROR",
				"error_message": "Some error message"
			}`)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
	})

	Convey("fetchTags SUCCESS", t, func(c C) {
		result, err := mockFetchTags(c, `{
				"status": "SUCCESS",
				"tags": [
					{
						"tag": "a:b1",
						"registered_by": "user:a@example.com",
						"registered_ts": "1420244414571500"
					},
					{
						"tag": "a:b2",
						"registered_by": "user:a@example.com",
						"registered_ts": "1420244414571500"
					}
				]
			}`, nil)
		So(err, ShouldBeNil)
		So(result, ShouldResemble, []TagInfo{
			{
				Tag:          "a:b1",
				RegisteredBy: "user:a@example.com",
				RegisteredTs: UnixTime(time.Unix(0, 1420244414571500000)),
			},
			{
				Tag:          "a:b2",
				RegisteredBy: "user:a@example.com",
				RegisteredTs: UnixTime(time.Unix(0, 1420244414571500000)),
			},
		})
	})

	Convey("fetchTags PACKAGE_NOT_FOUND", t, func(c C) {
		result, err := mockFetchTags(c, `{"status": "PACKAGE_NOT_FOUND"}`, nil)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
	})

	Convey("fetchTags INSTANCE_NOT_FOUND", t, func(c C) {
		result, err := mockFetchTags(c, `{"status": "INSTANCE_NOT_FOUND"}`, nil)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
	})

	Convey("fetchTags ERROR", t, func(c C) {
		result, err := mockFetchTags(c, `{
				"status": "ERROR",
				"error_message": "Some error message"
			}`, nil)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
	})

	Convey("fetchRefs SUCCESS", t, func(c C) {
		result, err := mockFetchRefs(c, `{
				"status": "SUCCESS",
				"refs": [
					{
						"ref": "ref1",
						"modified_by": "user:a@example.com",
						"modified_ts": "1420244414571500"
					},
					{
						"ref": "ref2",
						"modified_by": "user:a@example.com",
						"modified_ts": "1420244414571500"
					}
				]
			}`, nil)
		So(err, ShouldBeNil)
		So(result, ShouldResemble, []RefInfo{
			{
				Ref:        "ref1",
				ModifiedBy: "user:a@example.com",
				ModifiedTs: UnixTime(time.Unix(0, 1420244414571500000)),
			},
			{
				Ref:        "ref2",
				ModifiedBy: "user:a@example.com",
				ModifiedTs: UnixTime(time.Unix(0, 1420244414571500000)),
			},
		})
	})

	Convey("fetchRefs PACKAGE_NOT_FOUND", t, func(c C) {
		result, err := mockFetchRefs(c, `{"status": "PACKAGE_NOT_FOUND"}`, nil)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
	})

	Convey("fetchRefs INSTANCE_NOT_FOUND", t, func(c C) {
		result, err := mockFetchRefs(c, `{"status": "INSTANCE_NOT_FOUND"}`, nil)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
	})

	Convey("fetchRefs ERROR", t, func(c C) {
		result, err := mockFetchRefs(c, `{
				"status": "ERROR",
				"error_message": "Some error message"
			}`, nil)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
	})

	Convey("fetchACL SUCCESS", t, func(c C) {
		result, err := mockFetchACL(c, `{
				"status": "SUCCESS",
				"acls": {
					"acls": [
						{
							"package_path": "a",
							"role": "OWNER",
							"principals": ["user:a", "group:b"],
							"modified_by": "user:abc@example.com",
							"modified_ts": "1420244414571500"
						},
						{
							"package_path": "a/b",
							"role": "READER",
							"principals": ["group:c"],
							"modified_by": "user:abc@example.com",
							"modified_ts": "1420244414571500"
						}
					]
				}
			}`)
		So(err, ShouldBeNil)
		So(result, ShouldResemble, []PackageACL{
			{
				PackagePath: "a",
				Role:        "OWNER",
				Principals:  []string{"user:a", "group:b"},
				ModifiedBy:  "user:abc@example.com",
				ModifiedTs:  UnixTime(time.Unix(0, 1420244414571500000)),
			},
			{
				PackagePath: "a/b",
				Role:        "READER",
				Principals:  []string{"group:c"},
				ModifiedBy:  "user:abc@example.com",
				ModifiedTs:  UnixTime(time.Unix(0, 1420244414571500000)),
			},
		})
	})

	Convey("fetchACL ERROR", t, func(c C) {
		result, err := mockFetchACL(c, `{
				"status": "ERROR",
				"error_message": "Some error message"
			}`)
		So(err, ShouldNotBeNil)
		So(result, ShouldBeNil)
	})

	Convey("modifyACL SUCCESS", t, func(c C) {
		expected := `{
				"changes": [
					{
						"action": "GRANT",
						"role": "OWNER",
						"principal": "user:a@example.com"
					},
					{
						"action": "REVOKE",
						"role": "READER",
						"principal": "user:b@example.com"
					}
				]
			}`
		// Strip " ", "\t" and "\n".
		expected = strings.Replace(expected, " ", "", -1)
		expected = strings.Replace(expected, "\n", "", -1)
		expected = strings.Replace(expected, "\t", "", -1)

		err := mockModifyACL(c, []PackageACLChange{
			{
				Action:    GrantRole,
				Role:      "OWNER",
				Principal: "user:a@example.com",
			},
			{
				Action:    RevokeRole,
				Role:      "READER",
				Principal: "user:b@example.com",
			},
		}, expected, `{"status":"SUCCESS"}`)
		So(err, ShouldBeNil)
	})

	Convey("modifyACL ERROR", t, func(c C) {
		err := mockModifyACL(c, []PackageACLChange{}, `{"changes":null}`, `{
				"status": "ERROR",
				"error_message": "Error message"
			}`)
		So(err, ShouldNotBeNil)
	})

	Convey("setRef SUCCESS", t, func(c C) {
		So(mockSetRef(c, `{"status":"SUCCESS"}`), ShouldBeNil)
	})

	Convey("setRef bad ref", t, func(c C) {
		err := mockRemoteImpl(c, nil).setRef(
			ctx, "BAD REF",
			Pin{"pkgname", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
		So(err, ShouldNotBeNil)
	})

	Convey("setRef PROCESSING_NOT_FINISHED_YET", t, func(c C) {
		err := mockSetRef(c, `{"status":"PROCESSING_NOT_FINISHED_YET", "error_message":"Blah"}`)
		So(err, ShouldResemble, &pendingProcessingError{message: "Blah"})
	})

	Convey("setRef ERROR", t, func(c C) {
		So(mockSetRef(c, `{"status":"ERROR", "error_message":"Blah"}`), ShouldNotBeNil)
	})

	Convey("listPackages SUCCESS", t, func(c C) {
		pkgs, dirs, err := mockListPackages(c, `{
				"status": "SUCCESS",
				"packages": [
					"pkgpath/fake1",
					"pkgpath/fake2"
				],
				"directories": []
			}`)
		So(err, ShouldBeNil)
		So(pkgs, ShouldResemble, []string{
			"pkgpath/fake1",
			"pkgpath/fake2",
		})
		So(dirs, ShouldBeEmpty)
	})

	Convey("listPackages ERROR", t, func(c C) {
		pkgs, dirs, err := mockListPackages(c, `{
				"status": "ERROR",
				"error_message": "Some error message"
			}`)
		So(err, ShouldNotBeNil)
		So(pkgs, ShouldBeNil)
		So(dirs, ShouldBeNil)
	})

	Convey("searchInstances SUCCESS", t, func(c C) {
		pins, err := mockSearchInstances(c, `{
				"status": "SUCCESS",
				"instances": [
					{
						"package_name": "pkg1",
						"instance_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
					},
					{
						"package_name": "pkg2",
						"instance_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
					}
				]
			}`)
		So(err, ShouldBeNil)
		So(pins, ShouldResemble, PinSlice{
			{"pkg1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			{"pkg2", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		})
	})

	Convey("searchInstances ERROR", t, func(c C) {
		_, err := mockSearchInstances(c, `{
				"status": "ERROR",
				"error_message": "Some error message"
			}`)
		So(err, ShouldNotBeNil)
	})

	Convey("listInstances SUCCESS", t, func(c C) {
		result, err := mockListInstances(c, `{
				"status": "SUCCESS",
				"instances": [
					{
						"instance_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						"package_name": "pkgname",
						"registered_by": "user:a@example.com",
						"registered_ts": "1420244414571500"
					},
					{
						"instance_id": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
						"package_name": "pkgname",
						"registered_by": "user:b@example.com",
						"registered_ts": "1420244414571500"
					}
				],
				"cursor": "next_cursor"
			}`)
		So(err, ShouldBeNil)
		So(result, ShouldResemble, &listInstancesResponse{
			instances: []InstanceInfo{
				{
					Pin:          Pin{"pkgname", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					RegisteredBy: "user:a@example.com",
					RegisteredTs: UnixTime(time.Unix(0, 1420244414571500000)),
				},
				{
					Pin:          Pin{"pkgname", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
					RegisteredBy: "user:b@example.com",
					RegisteredTs: UnixTime(time.Unix(0, 1420244414571500000)),
				},
			},
			cursor: "next_cursor",
		})
	})

	Convey("listInstances PACKAGE_NOT_FOUND", t, func(c C) {
		_, err := mockListInstances(c, `{"status": "PACKAGE_NOT_FOUND"}`)
		So(err, ShouldNotBeNil)
	})

	Convey("listInstances ERROR", t, func(c C) {
		_, err := mockListInstances(c, `{
				"status": "ERROR",
				"error_message": "Some error message"
			}`)
		So(err, ShouldNotBeNil)
	})

	Convey("attachTags SUCCESS", t, func(c C) {
		err := mockAttachTags(
			c, []string{"tag1:value1", "tag2:value2"},
			`{"tags":["tag1:value1","tag2:value2"]}`,
			`{"status":"SUCCESS"}`)
		So(err, ShouldBeNil)
	})

	Convey("attachTags bad tag", t, func(c C) {
		err := mockRemoteImpl(c, nil).attachTags(
			ctx, Pin{"pkgname", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			[]string{"BADTAG"})
		So(err, ShouldNotBeNil)
	})

	Convey("attachTags PROCESSING_NOT_FINISHED_YET", t, func(c C) {
		err := mockAttachTags(
			c, []string{"tag1:value1", "tag2:value2"},
			`{"tags":["tag1:value1","tag2:value2"]}`,
			`{"status":"PROCESSING_NOT_FINISHED_YET", "error_message":"Blah"}`)
		So(err, ShouldResemble, &pendingProcessingError{message: "Blah"})
	})

	Convey("attachTags ERROR", t, func(c C) {
		err := mockAttachTags(
			c, []string{"tag1:value1", "tag2:value2"},
			`{"tags":["tag1:value1","tag2:value2"]}`,
			`{"status":"ERROR", "error_message":"Blah"}`)
		So(err, ShouldNotBeNil)
	})

	Convey("resolveVersion SUCCESS", t, func(c C) {
		pin, err := mockResolveVersion(c, `{
			"status": "SUCCESS",
			"instance_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}`)
		So(err, ShouldBeNil)
		So(pin, ShouldResemble, Pin{"pkgname", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	})

	Convey("resolveVersion SUCCESS and bad instance ID", t, func(c C) {
		_, err := mockResolveVersion(c, `{
			"status": "SUCCESS",
			"instance_id": "bad_id"
		}`)
		So(err, ShouldNotBeNil)
	})

	Convey("resolveVersion PACKAGE_NOT_FOUND", t, func(c C) {
		_, err := mockResolveVersion(c, `{"status": "PACKAGE_NOT_FOUND"}`)
		So(err, ShouldNotBeNil)
	})

	Convey("resolveVersion INSTANCE_NOT_FOUND", t, func(c C) {
		_, err := mockResolveVersion(c, `{"status": "INSTANCE_NOT_FOUND"}`)
		So(err, ShouldNotBeNil)
	})

	Convey("resolveVersion AMBIGUOUS_VERSION", t, func(c C) {
		_, err := mockResolveVersion(c, `{"status": "AMBIGUOUS_VERSION"}`)
		So(err, ShouldNotBeNil)
	})

	Convey("resolveVersion ERROR", t, func(c C) {
		_, err := mockResolveVersion(c, `{"status": "ERROR", "error_message":"Blah"}`)
		So(err, ShouldNotBeNil)
	})

	Convey("resolveVersion bad status", t, func(c C) {
		_, err := mockResolveVersion(c, `{"status": "HUH?"}`)
		So(err, ShouldNotBeNil)
	})
}

////////////////////////////////////////////////////////////////////////////////

func mockRemoteImpl(c C, expectations []expectedHTTPCall) *remoteImpl {
	client := mockClient(c, "", expectations)
	return &remoteImpl{
		serviceURL: client.ServiceURL,
		userAgent:  client.UserAgent,
		client:     client.AnonymousClient,
	}
}
