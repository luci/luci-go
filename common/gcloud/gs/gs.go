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

package gs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	gs "cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/errors"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

var (
	// ReadWriteScopes is the set of scopes needed for read/write Google Storage
	// access.
	ReadWriteScopes = []string{gs.ScopeReadWrite}

	// ReadOnlyScopes is the set of scopes needed for read/write Google Storage
	// read-only access.
	ReadOnlyScopes = []string{gs.ScopeReadOnly}
)

// Client abstracts functionality to connect with and use Google Storage from
// the actual Google Storage client.
//
// Non-production implementations are used primarily for testing.
type Client interface {
	// Close closes the connection to Google Storage.
	Close() error

	// Attrs retrieves Object attributes for a given path.
	Attrs(p Path) (*gs.ObjectAttrs, error)

	// Objects retrieves all object attributes for a given path.
	Objects(p Path) ([]*gs.ObjectAttrs, error)

	// SignedURL generates a google storage signed url for a given path.
	SignedURL(p Path, opts *gs.SignedURLOptions) (string, error)

	// NewReader instantiates a new Reader instance for the named bucket/path.
	//
	// The supplied offset must be >= 0, or else this function will panic.
	//
	// If the supplied length is <0, no upper byte bound will be set.
	//
	// The reader inherits the context of the client.
	NewReader(p Path, offset, length int64) (io.ReadCloser, error)

	// NewWriter instantiates a new Writer instance for the named bucket/path.
	//
	// The writer inherits the context of the client.
	NewWriter(p Path) (Writer, error)

	// Delete deletes the object at the specified path.
	//
	// If the object does not exist, it is considered a success.
	Delete(p Path) error

	// Rename renames an object from one path to another.
	//
	// NOTE: The object should be removed from its original path, but current
	// implementation uses two operations (Copy + Delete), so it may
	// occasionally fail.
	Rename(src, dst Path) error
}

// prodGSObject is an implementation of Client interface using the production
// Google Storage client.
type prodClient struct {
	// ctx is used for deadlines and logging. Also referenced by prodWriter.
	ctx context.Context
	// rt is the RoundTripper to use, or nil for the cloud service default.
	rt http.RoundTripper
	// baseClient is a basic Google Storage client instance. It is used for
	// operations that don't need custom header injections.
	baseClient *gs.Client
}

// NewProdClient creates a new Client instance that uses production Cloud
// Storage.
//
// The supplied RoundTripper will be used to make connections. If nil, the
// default HTTP client will be used.
//
// The given context is used for all logging and for overall deadline and
// cancellation. The client is no longer usable when this context is canceled.
func NewProdClient(ctx context.Context, rt http.RoundTripper) (Client, error) {
	c := prodClient{
		ctx: ctx,
		rt:  rt,
	}
	var err error
	c.baseClient, err = c.newClient()
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *prodClient) Close() error {
	return c.baseClient.Close()
}

func (c *prodClient) NewWriter(p Path) (Writer, error) {
	bucket, filename, err := splitPathErr(p)
	if err != nil {
		return nil, err
	}

	return &prodWriter{
		client:  c,
		bucket:  bucket,
		relpath: filename,
	}, nil
}

func (c *prodClient) Attrs(p Path) (*gs.ObjectAttrs, error) {
	obj, err := c.handleForPath(p)
	if err != nil {
		return nil, err
	}
	return obj.Attrs(c.ctx)
}

func (c *prodClient) Objects(p Path) ([]*gs.ObjectAttrs, error) {
	bkt := c.baseClient.Bucket(p.Bucket())
	query := &gs.Query{Prefix: p.Filename()}

	var attrs []*gs.ObjectAttrs
	it := bkt.Objects(c.ctx, query)
	for {
		attr, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		attrs = append(attrs, attr)
	}
	return attrs, nil
}

func (c *prodClient) SignedURL(p Path, opts *gs.SignedURLOptions) (string, error) {
	url, err := c.baseClient.Bucket(p.Bucket()).SignedURL(p.Filename(), opts)
	if err != nil {
		return "", err
	}
	return url, nil
}

func (c *prodClient) NewReader(p Path, offset, length int64) (io.ReadCloser, error) {
	if offset < 0 {
		panic(fmt.Errorf("offset (%d) must be >= 0", offset))
	}

	obj, err := c.handleForPath(p)
	if err != nil {
		return nil, err
	}
	return obj.NewRangeReader(c.ctx, offset, length)
}

func (c *prodClient) Rename(src, dst Path) error {
	srcObj, err := c.handleForPath(src)
	if err != nil {
		return fmt.Errorf("invalid source path: %s", err)
	}

	dstObj, err := c.handleForPath(dst)
	if err != nil {
		return fmt.Errorf("invalid destination path: %s", err)
	}

	// First stage: CopyTo
	err = retry.Retry(c.ctx, transient.Only(retry.Default), func() error {
		if _, err := dstObj.CopierFrom(srcObj).Run(c.ctx); err != nil {
			// The storage library doesn't return gs.ErrObjectNotExist when Delete
			// returns a 404. Catch that explicitly.
			// 403 errors are non-transient.
			if isNotFoundError(err) || isForbidden(err) {
				return err
			}

			// Assume all unexpected errors are transient.
			return transient.Tag.Apply(err)
		}
		return nil
	}, func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      d,
			"src":        src,
			"dst":        dst,
		}.Warningf(c.ctx, "Transient error copying GS file. Retrying...")
	})
	if err != nil {
		return err
	}

	// Second stage: Delete. This is not fatal.
	if err := c.deleteObject(srcObj); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"path":       src,
		}.Warningf(c.ctx, "(Non-fatal) Failed to delete source during rename.")
	}
	return nil
}

func (c *prodClient) Delete(p Path) error {
	dstObj, err := c.handleForPath(p)
	if err != nil {
		return fmt.Errorf("invalid path: %s", err)
	}

	return c.deleteObject(dstObj)
}

func (c *prodClient) deleteObject(o *gs.ObjectHandle) error {
	return retry.Retry(c.ctx, transient.Only(retry.Default), func() error {
		if err := o.Delete(c.ctx); err != nil {
			// The storage library doesn't return gs.ErrObjectNotExist when Delete
			// returns a 404. Catch that explicitly.
			if isNotFoundError(err) {
				// If the file wasn't found, then the delete "succeeded".
				return nil
			}

			// 403 errors are non-transient.
			if isForbidden(err) {
				return err
			}

			// Assume all unexpected errors are transient.
			return transient.Tag.Apply(err)
		}
		return nil
	}, func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      d,
		}.Warningf(c.ctx, "Transient error deleting file. Retrying...")
	})
}

func (c *prodClient) newClient() (*gs.Client, error) {
	var opts []option.ClientOption
	if c.rt != nil {
		opts = []option.ClientOption{
			option.WithHTTPClient(&http.Client{Transport: c.rt}),
		}
	}
	return gs.NewClient(c.ctx, opts...)
}

func (c *prodClient) handleForPath(p Path) (*gs.ObjectHandle, error) {
	bucket, filename, err := splitPathErr(p)
	if err != nil {
		return nil, err
	}
	return c.baseClient.Bucket(bucket).Object(filename), nil
}

func splitPathErr(p Path) (bucket, filename string, err error) {
	bucket, filename = p.Split()
	switch {
	case bucket == "":
		err = errors.New("path has no bucket")
	case filename == "":
		err = errors.New("path has no filename")
	}
	return
}

func isForbidden(err error) bool {
	if t, ok := err.(*googleapi.Error); ok {
		switch t.Code {
		case http.StatusForbidden:
			return true
		}
	}
	return false
}

func isNotFoundError(err error) bool {
	if err == gs.ErrObjectNotExist {
		return true
	}
	// The storage library doesn't return gs.ErrObjectNotExist when Delete
	// returns a 404. Catch that explicitly.
	if t, ok := err.(*googleapi.Error); ok {
		switch t.Code {
		case http.StatusNotFound:
			return true
		}
	}
	return false
}
