// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gs

import (
	"io"
	"net/http"
	"time"

	gaeauthClient "github.com/luci/luci-go/appengine/gaeauth/client"
	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
	"google.golang.org/cloud"
	gs "google.golang.org/cloud/storage"
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

// prodGSObject is an implementation of Client interface using the production
// Google Storage client.
type prodClient struct {
	context.Context

	// baseClient is a basic Google Storage client instance. It is used for
	// operations that don't need custom header injections.
	baseClient *gs.Client
}

// NewProdClient creates a new Client instance that uses production Cloud
// Storage.
func NewProdClient(ctx context.Context) (Client, error) {
	c := prodClient{
		Context: ctx,
	}

	var err error
	c.baseClient, err = c.newClient(nil)
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
	client, err := c.newClient(&o)
	if err != nil {
		return nil, err
	}
	return client.Bucket(bucket).Object(relpath).NewReader(c)
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

func (c *prodClient) newClient(o *Options) (*gs.Client, error) {
	// Get an Authenticator bound to the token scopes that we need for BigTable.
	a, err := gaeauthClient.Authenticator(c, []string{gs.ScopeReadWrite}, nil)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Cloud Storage authenticator.")
		return nil, errors.New("failed to create Cloud Storage authenticator")
	}

	// This is a hack. Unfortunately, it is necessary since the Cloud Storage API
	// doesn't support setting range request headers. This installation enables
	// us to request ranges from Cloud Storage objects, which is super useful for
	// range requests since we have an index.
	//
	// The Client construction logic is taken from here:
	// https://godoc.org/google.golang.org/cloud/internal/transport#NewHTTPClient
	//
	// We have to replicate the token source confguration b/c our only entry point
	// into header editing is the "cloud.WithClient", which preempts all of the
	// token source generation logic.
	client, err := a.Client()
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to generate Cloud Storage client.")
		return nil, err
	}
	if o != nil {
		rt := gsRoundTripper{
			RoundTripper: client.Transport,
			Options:      o,
		}
		client.Transport = &rt
	}
	gsc, err := gs.NewClient(c, cloud.WithBaseHTTP(client))
	if err != nil {
		return nil, err
	}

	return gsc, nil
}
