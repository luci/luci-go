// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package caching

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io/ioutil"
	"net/url"
	"strings"
	"time"

	"github.com/luci/luci-go/common/config"
	"golang.org/x/net/context"
)

const (
	defaultVersion = "_default"
)

// Cache implements a generic caching layer.
//
// The layer has no consistency expectations.
type Cache interface {
	// Store adds data to the cache. If value is nil, the cache should invalidate
	// the supplied key.
	Store(c context.Context, key string, expire time.Duration, value []byte)

	// Retrieve pulls data from the cache. If no data is available, nil will be
	// returned with no error.
	Retrieve(c context.Context, key string) []byte
}

// Options is the set of configuration options for the caching layer.
type Options struct {
	// Version is the version string, if any, to add to the keys to differentiate
	// it from any other versions. If empty, no version string will be prepended.
	Version string

	// Cache is the caching layer to use.
	Cache Cache

	// Expiration is the maximum amount of time that the configuration should be
	// retained before it must be refreshed.
	//
	// Due to implementation details of the cache layer, the configuration may be
	// retained for less time if necessary.
	Expiration time.Duration
}

// NewFilter constructs a new caching config.Filter function.
func NewFilter(o Options) config.Filter {
	return func(c context.Context, cfg config.Interface) config.Interface {
		return &gaeConfig{
			Context: c,
			Options: &o,
			inner:   cfg,
		}
	}
}

// gaeConfig implements a config.Interface that caches results in MemCache.
type gaeConfig struct {
	context.Context
	*Options

	inner config.Interface
}

func (cfg *gaeConfig) GetConfig(configSet, path string, hashOnly bool) (*config.Config, error) {
	// If we're doing hash-only lookup, we're okay with either full or hash-only
	// result. However, if we have to do the lookup, we will store the result in
	// a hash-only cache bucket.
	c := config.Config{}
	key := cfg.cacheKey("configs", "full", configSet, path)
	if cfg.retrieve(key, &c) {
		return &c, nil
	}

	if hashOnly {
		key = cfg.cacheKey("configs", "hashOnly", configSet, path)
		if cfg.retrieve(key, &c) {
			return &c, nil
		}
	}

	ic, err := cfg.inner.GetConfig(configSet, path, hashOnly)
	if err != nil {
		return nil, err
	}

	cfg.store(key, ic)
	return ic, nil
}

func (cfg *gaeConfig) GetConfigByHash(contentHash string) (string, error) {
	c := ""
	key := cfg.cacheKey("configsByHash", contentHash)
	if cfg.retrieve(key, &c) {
		return c, nil
	}

	c, err := cfg.inner.GetConfigByHash(contentHash)
	if err != nil {
		return "", err
	}

	cfg.store(key, c)
	return c, nil
}

func (cfg *gaeConfig) GetConfigSetLocation(configSet string) (*url.URL, error) {
	v := ""
	key := cfg.cacheKey("configSet", "location", configSet)
	if cfg.retrieve(key, &v) {
		u, err := url.Parse(v)
		if err != nil {
			return u, nil
		}
	}

	u, err := cfg.inner.GetConfigSetLocation(configSet)
	if err != nil {
		return nil, err
	}

	cfg.store(key, u.String())
	return u, nil
}

func (cfg *gaeConfig) GetProjectConfigs(path string, hashesOnly bool) ([]config.Config, error) {
	var c []config.Config
	key := cfg.cacheKey("projectConfigs", "full", path)
	if cfg.retrieve(key, &c) {
		return c, nil
	}

	if hashesOnly {
		key = cfg.cacheKey("projectConfigs", "hashesOnly", path)
	}

	c, err := cfg.inner.GetProjectConfigs(path, hashesOnly)
	if err != nil {
		return nil, err
	}

	cfg.store(key, c)
	return c, nil
}

func (cfg *gaeConfig) GetProjects() ([]config.Project, error) {
	p := []config.Project(nil)
	key := cfg.cacheKey("projects")
	if cfg.retrieve(key, &p) {
		return p, nil
	}

	p, err := cfg.inner.GetProjects()
	if err != nil {
		return nil, err
	}

	cfg.store(key, p)
	return p, nil
}

func (cfg *gaeConfig) GetRefConfigs(path string, hashesOnly bool) ([]config.Config, error) {
	c := []config.Config(nil)
	key := cfg.cacheKey("refConfigs", "full", path)
	if cfg.retrieve(key, &c) {
		return c, nil
	}

	if hashesOnly {
		key = cfg.cacheKey("refConfigs", "hashesOnly", path)
	}

	c, err := cfg.inner.GetRefConfigs(path, hashesOnly)
	if err != nil {
		return nil, err
	}

	cfg.store(key, c)
	return c, nil
}

func (cfg *gaeConfig) GetRefs(projectID string) ([]string, error) {
	var refs []string
	key := cfg.cacheKey("refs", projectID)
	if cfg.retrieve(key, &refs) {
		return refs, nil
	}

	refs, err := cfg.inner.GetRefs(projectID)
	if err != nil {
		return nil, err
	}

	cfg.store(key, refs)
	return refs, nil
}

func (cfg *gaeConfig) retrieve(key string, v interface{}) bool {
	if cfg.Cache == nil {
		return false
	}

	// Load the cache value.
	d := cfg.Cache.Retrieve(cfg, key)
	if d == nil {
		return false
	}

	// Unzip.
	zr, err := zlib.NewReader(bytes.NewBuffer(d))
	if err != nil {
		return false
	}
	defer zr.Close()

	rd, err := ioutil.ReadAll(zr)
	if err != nil {
		return false
	}

	// Unpack.
	if err := json.Unmarshal(rd, v); err != nil {
		return false
	}

	return true
}

func (cfg *gaeConfig) store(key string, v interface{}) {
	if cfg.Cache == nil {
		return
	}

	// Convert "v" to zlib-compressed JSON.
	d, err := json.Marshal(v)
	if err != nil {
		return
	}

	buf := bytes.Buffer{}
	w := zlib.NewWriter(&buf)
	_, err = w.Write(d)
	if err != nil {
		w.Close()
		return
	}
	if err := w.Close(); err != nil {
		return
	}

	cfg.Cache.Store(cfg, key, cfg.Expiration, buf.Bytes())
}

// cacheKey constructs a cache key from a set of value segments.
//
// In order to ensure that segments remain distinct in the resulting key, each
// segment is URL query escaped, and segments are joined by the non-escaped
// character, "|".
//
//   For example, ["a|b", "c"] => "a%7Cb|c"
func (cfg *gaeConfig) cacheKey(values ...string) string {
	version := cfg.Version
	if version == "" {
		version = defaultVersion
	}

	enc := url.QueryEscape
	parts := make([]string, 0, len(values)+1)
	parts = append(parts, enc(version))
	for _, v := range values {
		parts = append(parts, enc(v))
	}

	return strings.Join(parts, "|")
}
