// Copyright 2024 The LUCI Authors.
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

package gcemeta

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(response{}))
}

func TestTree(t *testing.T) {
	t.Parallel()

	root := &node{kind: kindDict}
	root.mount("/test/ok").dict(map[string]generator{
		"a": emit("a"),
		"gen": fast(func(_ context.Context, v url.Values) (any, error) {
			return v.Get("var"), nil
		}),
		"expensive": expensive(func(_ context.Context, v url.Values) (any, error) {
			return "zzz", nil
		}),
	})

	root.mount("/test/fail").dict(map[string]generator{
		"not-found": fast(func(context.Context, url.Values) (any, error) {
			return nil, errors.Reason("boom").Tag(statusTag(http.StatusNotFound)).Err()
		}),
		"transient": fast(func(context.Context, url.Values) (any, error) {
			return nil, errors.Reason("boom").Err()
		}),
	})

	var cases = []struct {
		title string
		url   string

		expectCT     string
		expectCode   int
		expectBody   string
		expectHeader map[string]string
	}{
		{
			title:      "Root listing 1",
			url:        "http://example.com",
			expectCT:   contentTypeText,
			expectCode: 200,
			expectBody: "test/\n",
		},
		{
			title:      "Root listing 2",
			url:        "http://example.com/",
			expectCT:   contentTypeText,
			expectCode: 200,
			expectBody: "test/\n",
		},
		{
			title:      "Non-root listing",
			url:        "http://example.com/test/",
			expectCT:   contentTypeText,
			expectCode: 200,
			expectBody: "fail/\nok/\n",
		},
		{
			title:        "Recursive JSON listing",
			url:          "http://example.com/test/ok/?recursive=true&var=zzz",
			expectCT:     contentTypeJSON,
			expectCode:   200,
			expectBody:   `{"a":"a","gen":"zzz"}`,
			expectHeader: map[string]string{"Etag": "058199eb2bb203ed"},
		},
		{
			title:        "Recursive text listing",
			url:          "http://example.com/test/ok/?recursive=true&var=zzz&alt=text",
			expectCT:     contentTypeText,
			expectCode:   200,
			expectBody:   "a a\ngen zzz\n",
			expectHeader: map[string]string{"Etag": "1d83d66ead2093f8"},
		},
		{
			title:        "Getting content",
			url:          "http://example.com/test/ok/a",
			expectCT:     contentTypeText,
			expectCode:   200,
			expectBody:   "a",
			expectHeader: map[string]string{"Etag": "ca978112ca1bbdca"},
		},
		{
			title:        "Getting slow content",
			url:          "http://example.com/test/ok/expensive",
			expectCT:     contentTypeText,
			expectCode:   200,
			expectBody:   "zzz",
			expectHeader: map[string]string{"Etag": ""}, // no Etag
		},
		{
			title:        "Getting content as JSON",
			url:          "http://example.com/test/ok/a?alt=json",
			expectCT:     contentTypeJSON,
			expectCode:   200,
			expectBody:   `"a"`,
			expectHeader: map[string]string{"Etag": "ac8d8342bbb2362d"},
		},
		{
			title:        "Redirect to the node listing",
			url:          "http://example.com/test/ok",
			expectCT:     "text/html",
			expectCode:   301,
			expectBody:   "/test/ok/\n",
			expectHeader: map[string]string{"Location": "http://example.com/test/ok/"},
		},
		{
			title:      "Unknown item",
			url:        "http://example.com/test/unknown",
			expectCT:   "text/plain; charset=utf-8",
			expectCode: 404,
			expectBody: `Metadata directory "/test" doesn't have "unknown"` + "\n",
		},
		{
			title:      "Traversing a leaf",
			url:        "http://example.com/test/ok/a/deeper",
			expectCT:   "text/plain; charset=utf-8",
			expectCode: 404,
			expectBody: `Metadata "/test/ok/a" can't be listed` + "\n",
		},
		{
			title:      "Listing a leaf",
			url:        "http://example.com/test/ok/a/",
			expectCT:   "text/plain; charset=utf-8",
			expectCode: 404,
			expectBody: `Metadata "/test/ok/a" can't be listed` + "\n",
		},
		{
			title:      "Failing item",
			url:        "http://example.com/test/fail/not-found",
			expectCT:   "text/plain; charset=utf-8",
			expectCode: 404,
			expectBody: "boom\n",
		},
		{
			title:      "Transient error",
			url:        "http://example.com/test/fail/transient",
			expectCT:   "text/plain; charset=utf-8",
			expectCode: 503,
			expectBody: "boom\n",
		},
		{
			title:      "alt=json for plain listing",
			url:        "http://example.com/test/?alt=json",
			expectCT:   "text/plain; charset=utf-8",
			expectCode: 400,
			expectBody: "Non-recursive directory listings do not support ?alt.\n",
		},
	}

	for _, cs := range cases {
		t.Run(cs.title, func(t *testing.T) {
			req := httptest.NewRequest("GET", cs.url, nil)
			req.Header.Add("Metadata-Flavor", "Google")
			resp := httptest.NewRecorder()
			serveMetadata(root, resp, req)
			hdr := resp.Result().Header
			assert.That(t, hdr.Get("Metadata-Flavor"), should.Equal("Google"))
			assert.That(t, hdr.Get("Server"), should.Equal("Metadata Server for LUCI"))
			assert.That(t, hdr.Get("X-Frame-Options"), should.Equal("SAMEORIGIN"))
			assert.That(t, hdr.Get("X-Xss-Protection"), should.Equal("0"))
			assert.That(t, resp.Code, should.Equal(cs.expectCode))
			assert.That(t, hdr.Get("Content-Type"), should.Equal(cs.expectCT))
			for k, v := range cs.expectHeader {
				assert.That(t, hdr.Get(k), should.Equal(v), truth.Explain("Key %q", k))
			}
			assert.That(t, resp.Body.String(), should.Equal(cs.expectBody))
		})
	}

	// Wrong method.
	req := httptest.NewRequest("POST", "http://example.com", nil)
	resp := httptest.NewRecorder()
	serveMetadata(root, resp, req)
	assert.That(t, resp.Code, should.Equal(405))
	assert.That(t, resp.Body.String(), should.Equal("Method not allowed\n"))

	// Missing required header.
	req = httptest.NewRequest("GET", "http://example.com", nil)
	resp = httptest.NewRecorder()
	serveMetadata(root, resp, req)
	assert.That(t, resp.Code, should.Equal(400))
	assert.That(t, resp.Body.String(), should.Equal("Bad Metadata-Flavor: got \"\", want \"Google\"\n"))

	// Forbidden header.
	req = httptest.NewRequest("GET", "http://example.com", nil)
	req.Header.Add("Metadata-Flavor", "Google")
	req.Header.Add("X-Forwarded-For", "zzz")
	resp = httptest.NewRecorder()
	serveMetadata(root, resp, req)
	assert.That(t, resp.Code, should.Equal(400))
	assert.That(t, resp.Body.String(), should.Equal("Forbidden X-Forwarded-For header \"zzz\"\n"))
}

func TestRecursiveListing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	root := &node{kind: kindDict}
	root.mount("/a").dict(map[string]generator{
		"int-leaf":        emit(123),
		"str-leaf":        emit("str"),
		"list-leaf":       emit([]string{"a", "b"}),
		"list-leaf-empty": emit([]string{}),
		"dict-leaf":       emit(map[string]string{"not-jsoninifed": "x", "b": "y"}),
		"dict-leaf-empty": emit(map[string]string{}),
		"expensive":       expensive(nil), // will not be called, can be nil
		"skipped": fast(func(context.Context, url.Values) (any, error) {
			return nil, errors.Reason("boom").Tag(statusTag(http.StatusNotFound)).Err()
		}),
	})

	root.mount("/a/list-node").list([]generator{emit("a"), emit("b")})
	root.mount("/a/list-node-empty").list([]generator{})

	root.mount("/a/dict-node").dict(map[string]generator{"a": emit("a")})
	root.mount("/a/dict-node-empty").dict(map[string]generator{})

	t.Run("JSON", func(t *testing.T) {
		resp, err := recursiveJSONListing(ctx, root, nil)
		assert.That(t, err, should.ErrLike(nil))
		assert.Loosely(t, resp, should.Match(&response{
			contentType: contentTypeJSON,
			body: []byte(
				`{"a":{"dictLeaf":{"b":"y","not-jsoninifed":"x"},` +
					`"dictLeafEmpty":{},` +
					`"dictNode":{"a":"a"},` +
					`"dictNodeEmpty":{},` +
					`"intLeaf":123,` +
					`"listLeaf":["a","b"],` +
					`"listLeafEmpty":[],` +
					`"listNode":["a","b"],` +
					`"listNodeEmpty":[],` +
					`"strLeaf":"str"}}`),
		}))
	})

	t.Run("Text", func(t *testing.T) {
		resp, err := recursiveTextListing(ctx, root, nil)
		assert.That(t, err, should.ErrLike(nil))
		assert.Loosely(t, resp, should.Match(&response{
			contentType: contentTypeText,
			body: []byte(
				"a/dict-leaf b y\n" +
					"a/dict-leaf not-jsoninifed x\n" +
					"a/dict-node/a a\n" +
					"a/int-leaf 123\n" +
					"a/list-leaf a\n" +
					"a/list-leaf b\n" +
					"a/list-node/0 a\n" +
					"a/list-node/1 b\n" +
					"a/str-leaf str\n"),
		}))
	})
}

func TestPlainListing(t *testing.T) {
	t.Parallel()

	n := &node{kind: kindDict}
	n.mount("/a/leaf").leaf(emit(123))
	n.mount("/a/more").dict(map[string]generator{"deeper": emit(123)})
	n.mount("/a/expensive").leaf(expensive(nil)) // will not be called, can be nil

	resp, err := plainListing(n.children["a"])
	assert.That(t, err, should.ErrLike(nil))
	assert.Loosely(t, resp, should.Match(&response{
		contentType: contentTypeText,
		body:        []byte("expensive\nleaf\nmore/\n"),
	}))

	resp, err = plainListing(n.children["a"].children["more"])
	assert.That(t, err, should.ErrLike(nil))
	assert.Loosely(t, resp, should.Match(&response{
		contentType: contentTypeText,
		body:        []byte("deeper\n"), // one-item listings have trailing "\n"
	}))
}

func TestNodeContent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("Non-leaf", func(t *testing.T) {
		resp, err := nodeContent(ctx, &node{kind: kindDict, path: "/some"}, nil, "")
		assert.That(t, err, should.ErrLike(nil))
		assert.Loosely(t, resp, should.Match(&response{
			redirect: "/some/",
		}))
	})

	t.Run("Map", func(t *testing.T) {
		n := &node{
			kind:    kindLeaf,
			content: emit(map[string]string{"a": "b", "c": "d"}),
		}

		// JSON by default.
		resp, err := nodeContent(ctx, n, nil, "")
		assert.That(t, err, should.ErrLike(nil))
		assert.Loosely(t, resp, should.Match(&response{
			contentType: contentTypeJSON,
			body:        []byte(`{"a":"b","c":"d"}`),
		}))

		// alt=text makes it text.
		resp, err = nodeContent(ctx, n, nil, "text")
		assert.That(t, err, should.ErrLike(nil))
		assert.Loosely(t, resp, should.Match(&response{
			contentType: contentTypeText,
			body:        []byte("a b\nc d\n"),
		}))
	})

	t.Run("List", func(t *testing.T) {
		n := &node{
			kind:    kindLeaf,
			content: emit([]string{"a", "b"}),
		}

		// Text by default.
		resp, err := nodeContent(ctx, n, nil, "")
		assert.That(t, err, should.ErrLike(nil))
		assert.Loosely(t, resp, should.Match(&response{
			contentType: contentTypeText,
			body:        []byte("a\nb\n"),
		}))

		// alt=json makes it JSON.
		resp, err = nodeContent(ctx, n, nil, "json")
		assert.That(t, err, should.ErrLike(nil))
		assert.Loosely(t, resp, should.Match(&response{
			contentType: contentTypeJSON,
			body:        []byte(`["a","b"]`),
		}))
	})

	t.Run("String", func(t *testing.T) {
		n := &node{
			kind:    kindLeaf,
			content: emit("abc"),
		}

		// Text by default. No trailing new line.
		resp, err := nodeContent(ctx, n, nil, "")
		assert.That(t, err, should.ErrLike(nil))
		assert.Loosely(t, resp, should.Match(&response{
			contentType: contentTypeText,
			body:        []byte("abc"),
		}))

		// alt=json makes it JSON.
		resp, err = nodeContent(ctx, n, nil, "json")
		assert.That(t, err, should.ErrLike(nil))
		assert.Loosely(t, resp, should.Match(&response{
			contentType: contentTypeJSON,
			body:        []byte(`"abc"`),
		}))
	})

	t.Run("Int", func(t *testing.T) {
		n := &node{
			kind:    kindLeaf,
			content: emit(123),
		}

		// Text and JSON look the same. Only content type is different.
		resp, err := nodeContent(ctx, n, nil, "")
		assert.That(t, err, should.ErrLike(nil))
		assert.Loosely(t, resp, should.Match(&response{
			contentType: contentTypeText,
			body:        []byte("123"),
		}))

		// alt=json makes it JSON content type.
		resp, err = nodeContent(ctx, n, nil, "json")
		assert.That(t, err, should.ErrLike(nil))
		assert.Loosely(t, resp, should.Match(&response{
			contentType: contentTypeJSON,
			body:        []byte("123"),
		}))
	})

	t.Run("Propagates err", func(t *testing.T) {
		testErr := errors.New("boom")

		n := &node{
			kind: kindLeaf,
			content: fast(func(context.Context, url.Values) (any, error) {
				return nil, testErr
			}),
		}

		_, err := nodeContent(ctx, n, nil, "")
		assert.That(t, err, should.Equal(testErr))
	})
}

func TestConvertToText(t *testing.T) {
	t.Parallel()

	ok := func(obj any, out []string) {
		res, err := convertToText(obj)
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, res, should.Match(out))
	}

	ok(nil, []string{"null"})
	ok(123, []string{"123"})
	ok(123.4, []string{"123.4"})
	ok("blah", []string{"blah"})
	ok([]int{1, 2}, []string{"1", "2"})
	ok([]string{"a", "b"}, []string{"a", "b"})
	ok(map[string]int{"a": 1, "b": 2}, []string{"a 1", "b 2"})
	ok(map[string]any{"a": []int{1}, "b": map[string]int{"z": 2}}, []string{"a [1]", `b {"z":2}`})
}

func TestJSONKey(t *testing.T) {
	t.Parallel()

	assert.That(t, jsonKey(""), should.Equal(""))
	assert.That(t, jsonKey("some"), should.Equal("some"))
	assert.That(t, jsonKey("some-more"), should.Equal("someMore"))
	assert.That(t, jsonKey("some-even-more"), should.Equal("someEvenMore"))
}
