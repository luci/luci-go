// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gs

import (
	"io"
	"net/http"
	"time"

	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
	"google.golang.org/cloud"
	gs "google.golang.org/cloud/storage"
)

var (
	// ReadWriteScopes is the set of scopes needed for read/write Google Storage
	// access.
	ReadWriteScopes = []string{gs.ScopeReadWrite}

	// ReadOnlyScopes is the set of scopes needed for read/write Google Storage
	// read-only access.
	ReadOnlyScopes = []string{gs.ScopeReadOnly}
)

// Client abstracts funcitonality to connect with and use Google Storage from
// the actual Google Storage client.
//
// Non-production implementations are used primarily for testing.
type Client interface {
	io.Closer

	// NewReader instantiates a new Reader instance for the named bucket/path.
	NewReader(bucket, relpath string, o Options) (io.ReadCloser, error)
	// NewWriter instantiates a new Writer instance for the named bucket/path.
	NewWriter(bucket, relpath string) (Writer, error)
	// Delete deletes the named Google Storage object. If the object doesn't
	// exist, a nil error will be returned.
	Delete(bucket, relpath string) error
}

// Options are the set of extra options to apply to the Google Storage request.
type Options struct {
	// From is the range request starting index. If >0, the beginning of the
	// range request will be set.
	From int64
	// To is the range request ending index. If >0, the end of the
	// range request will be set.
	//
	// If no From index is set, this will result in a request indexed from the end
	// of the object.
	To int64
}

// prodGSObject is an implementation of Client interface using the production
// Google Storage client.
type prodClient struct {
	context.Context

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
func NewProdClient(ctx context.Context, rt http.RoundTripper) (Client, error) {
	c := prodClient{
		Context: ctx,
		rt:      rt,
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

func (c *prodClient) NewWriter(bucket, relpath string) (Writer, error) {
	return &prodWriter{
		Context: c,
		client:  c,
		bucket:  bucket,
		relpath: relpath,
	}, nil
}

func (c *prodClient) NewReader(bucket, relpath string, o Options) (io.ReadCloser, error) {
	if o.From < 0 {
		o.From = 0
	}
	if o.To <= 0 {
		o.To = -1
	}
	return c.baseClient.Bucket(bucket).Object(relpath).NewRangeReader(c, o.From, o.To)
}

func (c *prodClient) Delete(bucket, relpath string) error {
	obj := c.baseClient.Bucket(bucket).Object(relpath)
	return retry.Retry(c, retry.TransientOnly(retry.Default), func() error {
		if err := obj.Delete(c); err != nil {
			// The storage library doesn't return gs.ErrObjectNotExist when Delete
			// returns a 404. Catch that explicitly.
			if t, ok := err.(*googleapi.Error); ok {
				switch t.Code {
				case http.StatusNotFound:
					// Delete failed because the object did not exist.
					return nil
				}
			}

			// Assume all unexpected errors are transient.
			return errors.WrapTransient(err)
		}
		return nil
	}, func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      d,
			"bucket":     bucket,
			"path":       relpath,
		}.Warningf(c, "Transient error deleting GS file. Retrying...")
	})
}

func (c *prodClient) newClient() (*gs.Client, error) {
	var optsArray [1]cloud.ClientOption
	opts := optsArray[:0]
	if c.rt != nil {
		opts = append(opts, cloud.WithBaseHTTP(&http.Client{
			Transport: c.rt,
		}))
	}
	return gs.NewClient(c, opts...)
}
