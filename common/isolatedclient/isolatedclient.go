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

package isolatedclient

import (
	"bufio"
	"bytes"
	"context"
	"crypto"
	"crypto/md5"
	"encoding/base64"
	"hash"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"strings"

	isolateservice "go.chromium.org/luci/common/api/isolate/isolateservice/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

// DefaultNamespace is the namespace that should be used with the New function.
const DefaultNamespace = "default-gzip"

// Source is a generator method to return source data. A generated Source must
// be Closed before the generator is called again.
type Source func() (io.ReadCloser, error)

// NewBytesSource returns a Source implementation that reads from the supplied
// byte slice.
func NewBytesSource(d []byte) Source {
	return func() (io.ReadCloser, error) {
		return ioutil.NopCloser(bytes.NewReader(d)), nil
	}
}

// PushState is per-item state passed from IsolateServer.Contains() to
// IsolateServer.Push().
//
// Its content is implementation specific.
type PushState struct {
	status    isolateservice.HandlersEndpointsV1PreuploadStatus
	digest    isolated.HexDigest
	size      int64
	uploaded  bool
	finalized bool
}

// CloudStorage is the interface for clients to fetch from and push to GCS storage.
type CloudStorage interface {
	// Fetch is a handler for retrieving specified content from GCS and storing
	// the response in the provided destination buffer.
	Fetch(context.Context, *Client, isolateservice.HandlersEndpointsV1RetrievedContent, io.Writer) error
	// Push is a handler for pushing content from provided buffer to GCS.
	Push(context.Context, *Client, isolateservice.HandlersEndpointsV1PreuploadStatus, Source) error
}

// Client is a client to an isolated server.
type Client struct {
	// All the members are immutable.
	retryFactory retry.Factory
	url          string

	// If you're unsure which namespace to use, use the DefaultNamespace constant.
	namespace string

	// The hashing algorithm used depends on the namespace.
	h crypto.Hash

	authClient *http.Client // client that sends auth tokens
	anonClient *http.Client // client that does NOT send auth tokens
	gcsHandler CloudStorage // implements GCS fetch and push handlers

	userAgent string
}

type Option func(*Client)

func WithNamespace(namespace string) Option {
	return func(i *Client) {
		i.namespace = namespace
	}
}

// WithAuthClient returns Option that sets client with authentication sufficient to talk to Isolate server
// (OAuth tokens with 'email' scope).
func WithAuthClient(c *http.Client) Option {
	return func(i *Client) {
		i.authClient = c
	}
}

// WithAnonymousClient returns Option that sets client which will be used by gcsHandler.
func WithAnonymousClient(c *http.Client) Option {
	return func(i *Client) {
		i.anonClient = c
	}
}

func WithRetryFactory(rFn retry.Factory) Option {
	return func(i *Client) {
		i.retryFactory = rFn
	}
}

func WithGCSHandler(gcs CloudStorage) Option {
	return func(i *Client) {
		i.gcsHandler = gcs
	}
}

func WithUserAgent(userAgent string) Option {
	return func(i *Client) {
		i.userAgent = userAgent
	}
}

// NewClient returns a new IsolateServer client.
func NewClient(host string, opts ...Option) *Client {
	i := &Client{
		url:        strings.TrimRight(host, "/"),
		namespace:  DefaultNamespace,
		authClient: http.DefaultClient,
		anonClient: http.DefaultClient,
		gcsHandler: defaultGCSHandler{},
	}

	for _, o := range opts {
		o(i)
	}

	i.h = isolated.GetHash(i.namespace)
	return i
}

// Hash returns the hashing algorithm used for this client.
func (i *Client) Hash() crypto.Hash {
	return i.h
}

// ServerCapabilities returns the server details.
func (i *Client) ServerCapabilities(c context.Context) (*isolateservice.HandlersEndpointsV1ServerDetails, error) {
	out := &isolateservice.HandlersEndpointsV1ServerDetails{}
	if err := i.postJSON(c, "/_ah/api/isolateservice/v1/server_details", nil, map[string]string{}, out); err != nil {
		return nil, err
	}
	return out, nil
}

// Contains looks up cache presence on the server of multiple items.
//
// The returned list is in the same order as 'items', with entries nil for
// items that were present.
func (i *Client) Contains(c context.Context, items []*isolateservice.HandlersEndpointsV1Digest) (out []*PushState, err error) {
	in := isolateservice.HandlersEndpointsV1DigestCollection{Items: items, Namespace: &isolateservice.HandlersEndpointsV1Namespace{}}
	in.Namespace.Namespace = i.namespace
	data := &isolateservice.HandlersEndpointsV1UrlCollection{}
	if err = i.postJSON(c, "/_ah/api/isolateservice/v1/preupload", nil, in, data); err != nil {
		return nil, err
	}
	out = make([]*PushState, len(items))
	for _, e := range data.Items {
		index := int(e.Index)
		out[index] = &PushState{
			status: *e,
			digest: isolated.HexDigest(items[index].Digest),
			size:   items[index].Size,
		}
	}
	return out, nil
}

// Push pushed a missing item, as reported by Contains(), to the server.
func (i *Client) Push(c context.Context, state *PushState, source Source) error {
	// This push operation may be a retry after failed finalization call below,
	// no need to reupload contents in that case.
	if !state.uploaded {
		// PUT file to uploadURL.
		if err := i.doPush(c, state, source); err != nil {
			log.Printf("doPush(%s) failed: %s\n%#v", state.digest, err, state)
			return err
		}
		state.uploaded = true
	}

	// Optionally notify the server that it's done.
	if state.status.GsUploadUrl != "" {
		// TODO(vadimsh): Calculate MD5 or CRC32C sum while uploading a file and
		// send it to isolated server. That way isolate server can verify that
		// the data safely reached Google Storage (GS provides MD5 and CRC32C of
		// stored files).
		in := isolateservice.HandlersEndpointsV1FinalizeRequest{UploadTicket: state.status.UploadTicket}
		headers := map[string]string{"Cache-Control": "public, max-age=31536000"}
		if err := i.postJSON(c, "/_ah/api/isolateservice/v1/finalize_gs_upload", headers, in, nil); err != nil {
			log.Printf("Push(%s) (finalize) failed: %s\n%#v", state.digest, err, state)
			return err
		}
	}
	state.finalized = true
	return nil
}

// Fetch downloads an item from the server.
func (i *Client) Fetch(c context.Context, digest isolated.HexDigest, dest io.Writer) error {
	// Perform initial request.
	in := &isolateservice.HandlersEndpointsV1RetrieveRequest{
		Digest: string(digest),
		Namespace: &isolateservice.HandlersEndpointsV1Namespace{
			Namespace: i.namespace,
		},
		Offset: 0,
	}
	var out isolateservice.HandlersEndpointsV1RetrievedContent

	if err := i.postJSON(c, "/_ah/api/isolateservice/v1/retrieve", nil, in, &out); err != nil {
		return err
	}

	// Handle DB items.
	if out.Content != "" {
		decoded, err := base64.StdEncoding.DecodeString(out.Content)
		if err != nil {
			return errors.Annotate(err, "failed to decode content").Err()
		}
		decompressor, err := isolated.GetDecompressor(i.namespace, bytes.NewReader(decoded))
		if err != nil {
			return errors.Annotate(err, "GetDecompressor failed").Err()
		}
		defer decompressor.Close()
		_, err = io.Copy(dest, decompressor)
		if err != nil {
			return errors.Annotate(err, "io.Copy failed").Tag(transient.Tag).Err()
		}
		return nil
	}

	// Handle GCS items.
	return i.gcsHandler.Fetch(c, i, out, dest)
}

// postJSON does authenticated POST request.
func (i *Client) postJSON(c context.Context, resource string, headers map[string]string, in, out interface{}) error {
	if len(resource) == 0 || resource[0] != '/' {
		return errors.Reason("resource must start with '/'").Err()
	}
	if i.userAgent != "" {
		// Clone headers.
		newheaders := map[string]string{}
		for k, v := range headers {
			newheaders[k] = v
		}
		headers = newheaders
		headers["User-Agent"] = i.userAgent
	}
	_, err := lhttp.PostJSON(c, i.retryFactory, i.authClient, i.url+resource, headers, in, out)
	return err
}

func (i *Client) doPush(c context.Context, state *PushState, source Source) (err error) {
	useDB := state.status.GsUploadUrl == ""
	if useDB {
		// Fast inline storage.
		var src io.ReadCloser
		if src, err = source(); err != nil {
			return err
		}
		defer src.Close()
		err = i.doPushDB(c, state, src)
	} else {
		// Storage is deferred to Google Cloud Storage.
		err = i.gcsHandler.Push(c, i, state.status, source)
	}

	return err
}

func (i *Client) doPushDB(c context.Context, state *PushState, reader io.Reader) error {
	buf := bytes.Buffer{}
	compressor, err := isolated.GetCompressor(i.namespace, &buf)
	if err != nil {
		return err
	}
	if _, err := io.Copy(compressor, reader); err != nil {
		return err
	}
	if err := compressor.Close(); err != nil {
		return err
	}
	in := &isolateservice.HandlersEndpointsV1StorageRequest{
		UploadTicket: state.status.UploadTicket,
		Content:      base64.StdEncoding.EncodeToString(buf.Bytes()),
	}
	return i.postJSON(c, "/_ah/api/isolateservice/v1/store_inline", nil, in, nil)
}

// defaultGCSHandler implements the default Fetch and Push handlers for
// interacting with GCS.
type defaultGCSHandler struct{}

// Fetch uses the provided HandlersEndpointsV1RetrievedContent response to
// download content from GCS to the provided dest.
func (defaultGCSHandler) Fetch(c context.Context, i *Client, content isolateservice.HandlersEndpointsV1RetrievedContent, dest io.Writer) error {
	rgen := func() (*http.Request, error) {
		return http.NewRequest("GET", content.Url, nil)
	}
	handler := func(resp *http.Response) error {
		defer resp.Body.Close()
		decompressor, err := isolated.GetDecompressor(i.namespace, resp.Body)
		if err != nil {
			annotator := errors.Annotate(err, "GCS GetDecompressor failed")
			if _, ok := err.(*net.OpError); ok {
				annotator.Tag(transient.Tag)
			}
			return annotator.Err()
		}
		defer decompressor.Close()
		_, err = io.Copy(dest, decompressor)
		if err != nil {
			return errors.Annotate(err, "GCS io.Copy failed").Tag(transient.Tag).Err()
		}
		return err
	}
	_, err := lhttp.NewRequest(c, i.anonClient, i.retryFactory, rgen, handler, nil)()
	return err
}

// Push uploads content from the provided source to the GCS path specified in
// the HandlersEndpointsV1PreuploadStatus response.
func (defaultGCSHandler) Push(ctx context.Context, i *Client, status isolateservice.HandlersEndpointsV1PreuploadStatus, source Source) error {
	// GsUploadUrl is signed Google Storage URL that doesn't require additional
	// authentication. In fact, using authClient causes HTTP 403 because
	// authClient's tokens don't have Cloud Storage OAuth scope. Use anonymous
	// client instead.
	var c *compressed
	req := lhttp.NewRequest(ctx, i.anonClient, i.retryFactory, func() (*http.Request, error) {
		src, err := source()
		if err != nil {
			return nil, err
		}

		request, err := http.NewRequest("PUT", status.GsUploadUrl, nil)
		if err != nil {
			src.Close()
			return nil, err
		}
		c = newCompressed(i.namespace, src)
		request.Body = c
		request.Header.Set("Content-Type", "application/octet-stream")
		return request, nil
	}, func(resp *http.Response) error {
		_, err4 := io.Copy(ioutil.Discard, resp.Body)
		err5 := resp.Body.Close()
		if err4 != nil {
			return err4
		}

		base64MD5 := "md5=" + base64.StdEncoding.EncodeToString(c.h.Sum(nil))
		xGoogHash := resp.Header["X-Goog-Hash"]
		verified := false
		for _, values := range xGoogHash {
			for _, value := range strings.Split(values, ",") {
				if value == base64MD5 {
					verified = true
					break
				}
			}
			if verified {
				break
			}
		}

		if !verified {
			return errors.Reason("hash mismatch, x-goog-hash=%q md5 hash=%q",
				xGoogHash, base64MD5).Tag(transient.Tag).Err()
		}

		return err5
	}, nil)
	_, err := req()
	return err
}

// compressed is an io.ReadCloser that transparently compresses source data in
// a separate goroutine.
type compressed struct {
	pr  *io.PipeReader
	src io.ReadCloser

	// Hold md5 checksum.
	h hash.Hash
}

func (c *compressed) Read(data []byte) (int, error) {
	return c.pr.Read(data)
}

func (c *compressed) Close() error {
	err := c.pr.Close()
	if err1 := c.src.Close(); err == nil {
		err = err1
	}
	return err
}

// newCompressed creates a pipeline to compress a file into a ReadCloser via:
// src (file as ReadCloser) -> gzip compressor (via io.CopyBuffer) -> bufio.Writer
// -> io.Pipe Writer side -> io.Pipe Reader side
// \
//  \-> hash.Hash writer for md5 hash checksum
func newCompressed(namespace string, src io.ReadCloser) *compressed {
	pr, pw := io.Pipe()
	h := md5.New()
	go func() {
		// Memory is cheap, we never want this pipeline to stall.
		const outBufSize = 1024 * 1024
		// We write into a buffer so that the Reader can read larger chunks of
		// compressed data out of the compressor instead of being constrained to
		// compressed(4096 bytes) each read.
		bufWriter := bufio.NewWriterSize(io.MultiWriter(pw, h), outBufSize)
		// The compressor itself is not thread safe.
		compressor, err := isolated.GetCompressor(namespace, bufWriter)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		// Make this 3x bigger than the output buffer since it is uncompressed.
		buf := make([]byte, outBufSize*3)
		if _, err := io.CopyBuffer(compressor, src, buf); err != nil {
			compressor.Close()
			pw.CloseWithError(err)
			return
		}
		// compressor needs to be closed first to flush the rest of the data
		// into the bufio.Writer
		if err := compressor.Close(); err != nil {
			pw.CloseWithError(err)
			return
		}
		pw.CloseWithError(bufWriter.Flush())
	}()

	return &compressed{pr, src, h}
}
