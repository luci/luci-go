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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
)

const (
	contentTypeText = "application/text"
	contentTypeJSON = "application/json"
)

// statusTag can be used to attach HTTP status code to the error.
//
// TODO: change the default to StatusServiceUnavailable and remove
// `statusCode()`
var statusTag = errtag.Make("metadata server status code", 0)

// statusCode extracts the status code from an error.
func statusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}
	code := http.StatusServiceUnavailable
	if val, ok := statusTag.Value(err); ok {
		code = val
	}
	return code
}

// node implements a node in the metadata tree.
//
// A node can either be "listed" or queried for its underlying contents. For
// example requesting "/computeMetadata/v1/instance/" would list the node, but
// requesting "/computeMetadata/v1/instance" (without trailing "/") would
// return its contents (which in this particular case would be a redirect).
//
// Leaf nodes can't be listed (attempting to do so results in HTTP 404). Their
// content is generated via the given `content` callback. Its exact type
// ("expensive" or "fast") determines if the leaf will be included in the
// recursive listing or not: only "fast" leafs are included. Leaf's content can
// be any JSON-serializable type. Maps and lists are recognized when generating
// text responses.
//
// Non-leaf nodes can be either dictionaries or lists. This defines how they are
// serialized when generating a recursive listing.
//
// See https://cloud.google.com/compute/docs/metadata/querying-metadata for some
// expected behaviors of the metadata server.
type node struct {
	kind     nodeKind         // what kind of node this is
	path     string           // e.g. "/computeMetadata/v1/instance"
	children map[string]*node // e.g. "<key>|<index> => ..." or nil map for leafs
	content  generator        // non-nil for leafs
}

// nodeKind defines how a node can be traversed and serialized.
type nodeKind string

const (
	kindDict = "dict" // holds <key> => *node mapping
	kindList = "list" // holds <index> => *node mapping
	kindLeaf = "leaf" // holds a leaf value
)

// generator is implemented by callbacks that can generate leaf's content.
type generator interface {
	expensive() bool
	invoke(ctx context.Context, q url.Values) (any, error)
}

// expensive is a type of content generators that are "expensive" to run.
//
// They are omitted from recursive listings.
type expensive func(ctx context.Context, q url.Values) (any, error)

func (f expensive) expensive() bool                                       { return true }
func (f expensive) invoke(ctx context.Context, q url.Values) (any, error) { return f(ctx, q) }

// fast is a type of content generates that are inexpensive to run.
//
// They are included in recursive listings.
type fast func(ctx context.Context, q url.Values) (any, error)

func (f fast) expensive() bool                                       { return false }
func (f fast) invoke(ctx context.Context, q url.Values) (any, error) { return f(ctx, q) }

// A fast generator that just returns the given value.
func emit[T any](val T) fast {
	return func(context.Context, url.Values) (any, error) {
		return val, nil
	}
}

// sortedKeys is a sorted slice of children keys.
func (n *node) sortedKeys() []string {
	keys := make([]string, 0, len(n.children))
	switch n.kind {
	case kindLeaf:
		// No children.
	case kindDict:
		// Order keys alphabetically.
		for k := range n.children {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	case kindList:
		// Order keys as integer indexes.
		for idx := 0; idx < len(n.children); idx++ {
			keys = append(keys, fmt.Sprintf("%d", idx))
		}
	}
	return keys
}

// child returns an immediate child of the node, creating it if necessary.
//
// A newly created node will have kindLeaf kind and nil `content`. The caller
// can adjusted them if necessary.
//
// Panics of `n` is a leaf node.
func (n *node) child(name string) (child *node, created bool) {
	if n.kind == kindLeaf {
		panic(fmt.Sprintf("trying to recurse into a leaf %q", n.path))
	}
	if child := n.children[name]; child != nil {
		return child, false
	}
	child = &node{
		kind: kindLeaf,
		path: fmt.Sprintf("%s/%s", n.path, name),
	}
	if n.children == nil {
		n.children = map[string]*node{name: child}
	} else {
		n.children[name] = child
	}
	return child, true
}

// mount creates a node at the given path, creating all intermediates if
// necessary.
//
// Panics when trying to recurse into leafs. Panics if the node already exists.
// A newly created node will have kindLeaf kind and nil `content`. The caller
// can adjusted them if necessary.
func (n *node) mount(path string) *node {
	elems := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(elems) == 0 {
		panic("empty path is not allowed here")
	}

	cur := n

	// Create all intermediate directories if necessary. This will traverse
	// through existing kindDict and kindList nodes.
	if len(elems) > 1 {
		for _, name := range elems[:len(elems)-1] {
			var created bool
			if cur, created = cur.child(name); created {
				cur.kind = kindDict // the default kind for intermediate nodes
			}
		}
	}

	// Add the final leaf. It must be absent.
	leaf, created := cur.child(elems[len(elems)-1])
	if !created {
		panic(fmt.Sprintf("node %q already exists", leaf.path))
	}
	return leaf
}

// dict converts an uninitialized leaf node into a kindDict node.
func (n *node) dict(leafs map[string]generator) {
	if n.kind != kindLeaf || n.content != nil || len(n.children) != 0 {
		panic("expecting uninitialized kindLeaf node")
	}
	n.kind = kindDict
	for key, val := range leafs {
		leaf, _ := n.child(key)
		leaf.content = val
	}
}

// list converts an uninitialized leaf node into a kindList node.
func (n *node) list(leafs []generator) {
	if n.kind != kindLeaf || n.content != nil || len(n.children) != 0 {
		panic("expecting uninitialized kindLeaf node")
	}
	n.kind = kindList
	for key, val := range leafs {
		leaf, _ := n.child(fmt.Sprintf("%d", key))
		leaf.content = val
	}
}

// leaf converts an initialized leaf node into an initialize leaf node.
func (n *node) leaf(gen generator) {
	if n.kind != kindLeaf || n.content != nil || len(n.children) != 0 {
		panic("expecting uninitialized kindLeaf node")
	}
	n.content = gen
}

// serveMetadata traverses the tree to generate the metadata response.
func serveMetadata(root *node, rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Metadata-Flavor", "Google")
	rw.Header().Set("Server", "Metadata Server for LUCI")
	rw.Header().Set("X-XSS-Protection", "0") // weird, but this is what the real server returns
	rw.Header().Set("X-Frame-Options", "SAMEORIGIN")

	resp, err := serveMetadataImpl(root, req)
	if err != nil {
		http.Error(rw, err.Error(), statusCode(err))
	} else {
		if resp.redirect != "" {
			// Reproduce the way the real metadata server does redirects.
			rw.Header().Set("Content-Type", "text/html")
			http.Redirect(rw, req,
				fmt.Sprintf("http://%s%s", req.Host, resp.redirect),
				http.StatusMovedPermanently,
			)
			_, _ = rw.Write([]byte(resp.redirect + "\n"))
		} else {
			rw.Header().Set("Content-Type", resp.contentType)
			if !resp.skipEtag {
				rw.Header().Set("Etag", genEtag(resp.body))
			}
			_, _ = rw.Write(resp.body)
		}
	}
}

// genEtag generates a hash of the content.
//
// It has a particular format to match the real metadata. The actual hashing
// algorithm is likely different though.
func genEtag(body []byte) string {
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:8])
}

// Written into the ResponseWriter.
type response struct {
	redirect    string
	contentType string
	body        []byte
	skipEtag    bool
}

// serveMetadataImpl actually implements the handler.
func serveMetadataImpl(root *node, req *http.Request) (*response, error) {
	// See https://cloud.google.com/compute/docs/metadata/querying-metadata
	if req.Method != "GET" {
		return nil, statusTag.ApplyValue(
			errors.New("Method not allowed"),
			http.StatusMethodNotAllowed)
	}
	if fl := req.Header.Get("Metadata-Flavor"); fl != "Google" {
		return nil, statusTag.ApplyValue(errors.Fmt("Bad Metadata-Flavor: got %q, want %q", fl, "Google"), http.StatusBadRequest)
	}
	if ff := req.Header.Get("X-Forwarded-For"); ff != "" {
		return nil, statusTag.ApplyValue(errors.Fmt("Forbidden X-Forwarded-For header %q", ff), http.StatusBadRequest)
	}

	// Normalize the path to be "/something" (or "/" for the root).
	path := req.URL.Path
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Trailing "/" indicate we want a listing. In particular the root ("/") is
	// always listed.
	listing := strings.HasSuffix(path, "/")

	// Detect "?recursive=...". The real GCE metadata server accepts all these.
	val := strings.ToLower(req.URL.Query().Get("recursive"))
	recursive := val == "1" || val == "true" || val == "yes"

	// The preferred response format can be set via "alt".
	alt := req.URL.Query().Get("alt")
	if listing && !recursive && alt != "" {
		// This is what the real GCE metadata server does as well.
		return nil, statusTag.ApplyValue(
			errors.New("Non-recursive directory listings do not support ?alt."),
			http.StatusBadRequest)
	}

	// Normalize the path to be a list of node names. This will be e.g. `[]` for
	// the root or `["computeMetadata", "v1", "instance"]` for a directory or
	// a leaf.
	elems := strings.Split(strings.Trim(path, "/"), "/")
	if len(elems) == 1 && elems[0] == "" {
		elems = nil // happens when `path` is "" or "/", want the root in that case
	}

	// Find the corresponding node.
	cur := root
	for _, name := range elems {
		if cur.kind == kindLeaf {
			return nil, statusTag.ApplyValue(errors.Fmt("Metadata %q can't be listed", cur.path), http.StatusNotFound)
		}
		next := cur.children[name]
		if next == nil {
			return nil, statusTag.ApplyValue(errors.Fmt("Metadata directory %q doesn't have %q", cur.path, name), http.StatusNotFound)
		}
		cur = next
	}

	if listing {
		if cur.kind == kindLeaf {
			return nil, statusTag.ApplyValue(errors.Fmt("Metadata %q can't be listed", cur.path), http.StatusNotFound)
		}
		switch {
		case recursive && alt == "text":
			return recursiveTextListing(req.Context(), cur, req.URL.Query())
		case recursive:
			return recursiveJSONListing(req.Context(), cur, req.URL.Query())
		default:
			return plainListing(cur)
		}
	}

	return nodeContent(req.Context(), cur, req.URL.Query(), alt)
}

// recursiveTextListing produces the recursive node listing as a list of lines.
//
// Does not list "expensive" leafs, only "fast" ones.
func recursiveTextListing(ctx context.Context, n *node, q url.Values) (*response, error) {
	var out []string
	root := n.path + "/"

	var traverse func(leaf *node) error

	traverse = func(n *node) error {
		if n.kind != kindLeaf {
			for _, key := range n.sortedKeys() {
				if err := traverse(n.children[key]); err != nil {
					return err
				}
			}
			return nil
		}
		if n.content.expensive() {
			return nil
		}
		switch obj, err := n.content.invoke(ctx, q); {
		case statusCode(err) == http.StatusNotFound:
			return nil
		case err != nil:
			return errors.Fmt("%s: %w", n.path, err)
		default:
			lines, err := convertToText(obj)
			if err != nil {
				return errors.Fmt("%s: %w", n.path, err)
			}
			if !strings.HasPrefix(n.path, root) {
				panic(fmt.Sprintf("impossible %q vs %q", n.path, root))
			}
			relPath := n.path[len(root):]
			for _, line := range lines {
				out = append(out, fmt.Sprintf("%s %s", relPath, line))
			}
			return nil
		}
	}

	err := traverse(n)
	if err != nil {
		return nil, err
	}
	return &response{
		contentType: contentTypeText,
		body:        []byte(strings.Join(out, "\n") + "\n"),
	}, nil
}

// recursiveJSONListing produces the recursive node listing as a JSON object.
//
// Does not list "expensive" leafs, only "fast" ones.
func recursiveJSONListing(ctx context.Context, n *node, q url.Values) (*response, error) {
	var traverse func(n *node) (any, error)

	errSkip := errors.New("skip this leaf")

	traverse = func(n *node) (any, error) {
		switch n.kind {
		case kindDict:
			out := make(map[string]any, len(n.children))
			for k, v := range n.children {
				switch obj, err := traverse(v); {
				case err == errSkip:
				case err != nil:
					return nil, err
				default:
					out[jsonKey(k)] = obj
				}
			}
			return out, nil

		case kindList:
			out := make([]any, len(n.children))
			for i, k := range n.sortedKeys() {
				switch obj, err := traverse(n.children[k]); {
				case err == errSkip:
				case err != nil:
					return nil, err
				default:
					out[i] = obj
				}
			}
			return out, nil

		case kindLeaf:
			if n.content.expensive() {
				return nil, errSkip
			}
			switch obj, err := n.content.invoke(ctx, q); {
			case statusCode(err) == http.StatusNotFound:
				return nil, errSkip
			case err != nil:
				return nil, errors.Fmt("%s: %w", n.path, err)
			default:
				return obj, nil
			}

		default:
			panic("impossible")
		}
	}

	out, err := traverse(n)
	if err != nil {
		return nil, err
	}

	blob, err := json.Marshal(out)
	if err != nil {
		return nil, errors.Fmt("converting to JSON: %w", err)
	}
	return &response{
		contentType: contentTypeJSON,
		body:        blob,
	}, nil
}

// jsonKey converts "key-like-this" into "keyLikeThis" for JSON output.
func jsonKey(k string) (out string) {
	for i, chunk := range strings.Split(k, "-") {
		if i != 0 && len(chunk) > 0 {
			// Assume ASCII metadata keys. "key" => "Key".
			chunk = strings.ToUpper(chunk[:1]) + chunk[1:]
		}
		out += chunk
	}
	return
}

// plainListing assembles a non-recursive node listing as a plain text.
//
// Lists "expensive" leafs as well as "fast" ones.
func plainListing(n *node) (*response, error) {
	lines := make([]string, 0, len(n.children))
	for _, k := range n.sortedKeys() {
		v := n.children[k]
		if v.kind == kindLeaf {
			lines = append(lines, k)
		} else {
			lines = append(lines, k+"/")
		}
	}
	return &response{
		contentType: contentTypeText,
		body:        []byte(strings.Join(lines, "\n") + "\n"),
	}, nil
}

// nodeContent generates and serializes leaf node content.
func nodeContent(ctx context.Context, n *node, q url.Values, alt string) (*response, error) {
	// Directory's content is a **redirect** to its listing.
	if n.kind != kindLeaf {
		return &response{redirect: n.path + "/"}, nil
	}

	// Ask the leaf node to generate its content.
	obj, err := n.content.invoke(ctx, q)
	if err != nil {
		return nil, err
	}

	// Etags are generated only for "fast" content, e.g. on the real GCE "/token"
	// endpoint doesn't have Etag.
	skipEtag := n.content.expensive()

	// Prefer JSON by default for dicts and text for everything else. If "alt" is
	// given, look if it specifically asks for an opposite of the preferred
	// format (and ignore it if says some nonsense). This weird logic of
	// recognizing only the opposite is how the real GCE metadata server behaves.
	format := "text"
	if reflect.TypeOf(obj).Kind() == reflect.Map {
		format = "json"
	}
	if format == "json" && alt == "text" {
		format = "text"
	} else if format == "text" && alt == "json" {
		format = "json"
	}

	switch format {
	case "json":
		blob, err := json.Marshal(obj)
		if err != nil {
			return nil, errors.Fmt("converting to JSON: %w", err)
		}
		return &response{
			contentType: contentTypeJSON,
			body:        blob,
			skipEtag:    skipEtag,
		}, nil

	case "text":
		lines, err := convertToText(obj)
		if err != nil {
			return nil, err
		}
		var body []byte
		if len(lines) == 1 {
			body = []byte(lines[0]) // no trailing "\n" for a single line
		} else {
			body = []byte(strings.Join(lines, "\n") + "\n") // add trailing "\n" for lists
		}
		return &response{
			contentType: contentTypeText,
			body:        body,
			skipEtag:    skipEtag,
		}, nil

	default:
		panic("impossible")
	}
}

// convertToText converts a leafs content into a list of lines.
//
// Only supports primitive types, lists of primitive types and non-nested maps
// with string keys. Anything more complex will be returned as inline JSON.
func convertToText(obj any) ([]string, error) {
	// Piggyback on JSON encoder for traversing `obj` and stringfying its values.
	blob, err := json.Marshal(obj)
	if err != nil {
		return nil, errors.Fmt("converting to JSON: %w", err)
	}

	toPlainText := func(msg json.RawMessage) string {
		if len(msg) == 0 {
			return ""
		}
		switch msg[0] {
		case '{', '[':
			// Nested structured values aren't supported. Dump them as JSON lines.
			return string(msg)
		case '"':
			// Get back an unescaped string.
			var str string
			if err := json.Unmarshal(msg, &str); err != nil {
				panic(err)
			}
			return str
		default:
			// E.g. an integer, a float, etc. Its JSON representation is already OK.
			return string(msg)
		}
	}

	switch blob[0] {
	case '{':
		// Convert the map to a list of "key value" lines.
		var m map[string]json.RawMessage
		if err := json.Unmarshal(blob, &m); err != nil {
			panic(err)
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]string, len(keys))
		for i, k := range keys {
			out[i] = fmt.Sprintf("%s %s", k, toPlainText(m[k]))
		}
		return out, nil

	case '[':
		// Convert a list to a list of plain text values.
		var lst []json.RawMessage
		if err := json.Unmarshal(blob, &lst); err != nil {
			panic(err)
		}
		out := make([]string, len(lst))
		for i, v := range lst {
			out[i] = toPlainText(v)
		}
		return out, nil

	default:
		// E.g. an integer, a float, a string, etc. A single line.
		return []string{toPlainText(blob)}, nil
	}
}
