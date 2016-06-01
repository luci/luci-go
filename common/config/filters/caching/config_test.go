// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package caching

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/config/impl/memory"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

type testCache struct {
	data     map[string][]byte
	callback func(hit bool)
}

func (tc *testCache) Store(c context.Context, key string, expire time.Duration, value []byte) {
	if tc.data == nil {
		tc.data = map[string][]byte{}
	}
	tc.data[key] = value
}

func (tc *testCache) Retrieve(c context.Context, key string) []byte {
	d, ok := tc.data[key]
	if tc.callback != nil {
		tc.callback(ok)
	}
	return d
}

func (tc *testCache) invalidate() {
	tc.data = nil
}

func (tc *testCache) nothingCached() bool {
	return len(tc.data) == 0
}

type forceErrConfig struct {
	inner config.Interface
	err   error
}

func (tc *forceErrConfig) ServiceURL() url.URL {
	return tc.inner.ServiceURL()
}

func (tc *forceErrConfig) GetConfig(configSet, path string, hashOnly bool) (*config.Config, error) {
	if tc.err != nil {
		return nil, tc.err
	}
	return tc.inner.GetConfig(configSet, path, hashOnly)
}

func (tc *forceErrConfig) GetConfigByHash(contentHash string) (string, error) {
	if tc.err != nil {
		return "", tc.err
	}
	return tc.inner.GetConfigByHash(contentHash)
}

func (tc *forceErrConfig) GetConfigSetLocation(configSet string) (*url.URL, error) {
	if tc.err != nil {
		return nil, tc.err
	}
	return tc.inner.GetConfigSetLocation(configSet)
}

func (tc *forceErrConfig) GetProjectConfigs(path string, hashesOnly bool) ([]config.Config, error) {
	if tc.err != nil {
		return nil, tc.err
	}
	return tc.inner.GetProjectConfigs(path, hashesOnly)
}

func (tc *forceErrConfig) GetProjects() ([]config.Project, error) {
	if tc.err != nil {
		return nil, tc.err
	}
	return tc.inner.GetProjects()
}

func (tc *forceErrConfig) GetRefConfigs(path string, hashesOnly bool) ([]config.Config, error) {
	if tc.err != nil {
		return nil, tc.err
	}
	return tc.inner.GetRefConfigs(path, hashesOnly)
}

func (tc *forceErrConfig) GetRefs(projectID string) ([]string, error) {
	if tc.err != nil {
		return nil, tc.err
	}
	return tc.inner.GetRefs(projectID)
}

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey(`A cache backed by a memory Config`, t, func() {
		c := context.Background()
		mbase := map[string]memory.ConfigSet{
			"services/foo": {
				"file": "body",
			},
			"projects/proj1": {
				"file": "project1 file",
			},
			"projects/goesaway": {
				"file": "project1 file",
			},
			"projects/goesaway/refs/heads/master": {
				"file": "goesaway master ref",
			},
			"projects/goesaway/refs/heads/other": {
				"file": "goesaway other ref",
			},
		}
		mcfg := memory.New(mbase)
		errCfg := &forceErrConfig{inner: mcfg}
		c = config.Set(c, errCfg)

		wasHit := false
		tc := testCache{
			callback: func(hit bool) { wasHit = hit },
		}
		opts := Options{
			Cache: &tc,
		}
		c = config.AddFilters(c, NewFilter(opts))
		cfg := config.Get(c)

		// Advance underlying config, expectation.
		advance := func() {
			mbase["services/foo"]["file"] = "body2"
			delete(mbase, "projects/goesaway")
			delete(mbase, "projects/goesaway/refs/heads/master")
			delete(mbase, "projects/goesaway/refs/heads/other")
		}

		Convey(`GetConfig`, func() {
			Convey(`A non-hash-only query retrieves and caches the full body.`, func() {
				exp, _ := mcfg.GetConfig("services/foo", "file", false)

				v, err := cfg.GetConfig("services/foo", "file", false)
				So(err, ShouldBeNil)
				So(v, ShouldResemble, exp)

				advance()
				v, err = cfg.GetConfig("services/foo", "file", false)
				So(err, ShouldBeNil)
				So(v, ShouldResemble, exp)

				Convey(`A hash-only query will use non-hash version.`, func() {
					v, err = cfg.GetConfig("services/foo", "file", true)
					So(err, ShouldBeNil)
					So(v, ShouldResemble, exp)
				})

				Convey(`A non-hash-only query caches by hash too.`, func() {
					byHash, err := cfg.GetConfigByHash(v.ContentHash)
					So(err, ShouldBeNil)
					So(byHash, ShouldResemble, v.Content)
				})
			})

			Convey(`A hash-only query will cache the hashed version.`, func() {
				exp, _ := mcfg.GetConfig("services/foo", "file", true)

				v, err := cfg.GetConfig("services/foo", "file", true)
				So(err, ShouldBeNil)
				So(v, ShouldResemble, exp)

				advance()
				v, err = cfg.GetConfig("services/foo", "file", true)
				So(err, ShouldBeNil)
				So(v, ShouldResemble, exp)

				Convey(`A full query will not use the cached hash-only version.`, func() {
					exp, _ := mcfg.GetConfig("services/foo", "file", false)

					v, err = cfg.GetConfig("services/foo", "file", false)
					So(err, ShouldBeNil)
					So(v, ShouldResemble, exp)
				})

				Convey(`A hash-only query does not cache the full config by hash.`, func() {
					_, err := cfg.GetConfigByHash(v.ContentHash)
					So(err, ShouldNotBeNil)
				})
			})

			Convey(`GetConfig missing will be cached`, func() {
				_, err := cfg.GetConfig("services/invalid", "file", false)
				So(err, ShouldEqual, config.ErrNoConfig)
				So(wasHit, ShouldBeFalse)

				_, err = cfg.GetConfig("services/invalid", "file", false)
				So(err, ShouldEqual, config.ErrNoConfig)
				So(wasHit, ShouldBeTrue)
			})

			Convey(`Other errors will not be cached`, func() {
				errCfg.err = errors.New("test error")
				_, err := cfg.GetConfig("services/foo", "file", false)
				So(err, ShouldErrLike, "test error")
				So(tc.nothingCached(), ShouldBeTrue)
			})
		})

		Convey(`GetConfigByHash`, func() {
			Convey(`Works.`, func() {
				body, err := cfg.GetConfigByHash("v1:fb4c35e739d53994aba7d3e0416a1082f11bfbba")
				So(err, ShouldBeNil)
				So(body, ShouldResemble, "body")

				advance()
				body, err = cfg.GetConfigByHash("v1:fb4c35e739d53994aba7d3e0416a1082f11bfbba")
				So(err, ShouldBeNil)
				So(body, ShouldResemble, "body")
			})

			Convey(`Missing caches and returns error`, func() {
				_, err := cfg.GetConfigByHash("v0:asdf")
				So(err, ShouldEqual, config.ErrNoConfig)
				So(wasHit, ShouldBeFalse)

				_, err = cfg.GetConfigByHash("v0:asdf")
				So(err, ShouldEqual, config.ErrNoConfig)
				So(wasHit, ShouldBeTrue)
			})

			Convey(`Other errors will not be cached`, func() {
				errCfg.err = errors.New("test error")
				_, err := cfg.GetConfigByHash("v0:asdf")
				So(err, ShouldErrLike, "test error")
				So(tc.nothingCached(), ShouldBeTrue)
			})
		})

		Convey(`GetConfigSetLocation`, func() {
			Convey(`Works.`, func() {
				loc, err := cfg.GetConfigSetLocation("projects/goesaway")
				So(err, ShouldBeNil)
				So(loc, ShouldNotBeNil)

				advance()
				loc2, err := cfg.GetConfigSetLocation("projects/goesaway")
				So(err, ShouldBeNil)
				So(loc2, ShouldResemble, loc)
			})

			Convey(`Missing caches and returns error`, func() {
				errCfg.err = config.ErrNoConfig
				_, err := cfg.GetConfigSetLocation("projects/goesaway")
				So(err, ShouldEqual, config.ErrNoConfig)
				So(wasHit, ShouldBeFalse)

				_, err = cfg.GetConfigSetLocation("projects/goesaway")
				So(err, ShouldEqual, config.ErrNoConfig)
				So(wasHit, ShouldBeTrue)
			})

			Convey(`Other errors will not be cached`, func() {
				errCfg.err = errors.New("test error")
				_, err := cfg.GetConfigSetLocation("projects/goesaway")
				So(err, ShouldErrLike, "test error")
				So(tc.nothingCached(), ShouldBeTrue)
			})
		})

		Convey(`GetProjectConfigs`, func() {
			Convey(`Works.`, func() {
				exp, _ := mcfg.GetProjectConfigs("file", false)

				cfgs, err := cfg.GetProjectConfigs("file", false)
				So(err, ShouldBeNil)
				So(cfgs, ShouldResemble, exp)

				advance()
				cfgs, err = cfg.GetProjectConfigs("file", false)
				So(err, ShouldBeNil)
				So(cfgs, ShouldResemble, exp)

				Convey(`Hash-only will returned cached full content.`, func() {
					cfgs, err = cfg.GetProjectConfigs("file", true)
					So(err, ShouldBeNil)
					So(cfgs, ShouldResemble, exp)
				})
			})

			Convey(`Hash-only works.`, func() {
				exp, _ := mcfg.GetProjectConfigs("file", true)

				cfgs, err := cfg.GetProjectConfigs("file", true)
				So(err, ShouldBeNil)
				So(cfgs, ShouldResemble, exp)

				Convey(`Full query will not use hash-only cached version.`, func() {
					advance()
					exp, _ := mcfg.GetProjectConfigs("file", false)

					cfgs, err := cfg.GetProjectConfigs("file", false)
					So(err, ShouldBeNil)
					So(cfgs, ShouldResemble, exp)
				})
			})
		})

		Convey(`GetProjects`, func() {
			Convey(`Works.`, func() {
				exp, _ := mcfg.GetProjects()
				proj, err := cfg.GetProjects()
				So(err, ShouldBeNil)
				So(proj, ShouldResemble, exp)

				advance()
				exp2, _ := mcfg.GetProjects()
				proj, err = cfg.GetProjects()
				So(err, ShouldBeNil)
				So(proj, ShouldResemble, exp)
				So(proj, ShouldNotResemble, exp2)
			})
		})

		Convey(`GetRefConfigs`, func() {
			Convey(`Works.`, func() {
				exp, _ := mcfg.GetRefConfigs("file", false)

				cfgs, err := cfg.GetRefConfigs("file", false)
				So(err, ShouldBeNil)
				So(cfgs, ShouldResemble, exp)

				advance()
				cfgs, err = cfg.GetRefConfigs("file", false)
				So(err, ShouldBeNil)
				So(cfgs, ShouldResemble, exp)

				Convey(`Hash-only will returned cached full content.`, func() {
					cfgs, err = cfg.GetRefConfigs("file", true)
					So(err, ShouldBeNil)
					So(cfgs, ShouldResemble, exp)
				})
			})

			Convey(`Hash-only works.`, func() {
				exp, _ := mcfg.GetRefConfigs("file", true)

				cfgs, err := cfg.GetRefConfigs("file", true)
				So(err, ShouldBeNil)
				So(cfgs, ShouldResemble, exp)

				Convey(`Full query will not use hash-only cached version.`, func() {
					advance()
					exp, _ := mcfg.GetRefConfigs("file", false)

					cfgs, err := cfg.GetRefConfigs("file", false)
					So(err, ShouldBeNil)
					So(cfgs, ShouldResemble, exp)
				})
			})
		})

		Convey(`GetRefs`, func() {
			Convey(`Works.`, func() {
				exp, _ := mcfg.GetRefs("goesaway")
				proj, err := cfg.GetRefs("goesaway")
				So(err, ShouldBeNil)
				So(proj, ShouldResemble, exp)

				advance()
				exp2, _ := mcfg.GetRefs("goesaway")
				proj, err = cfg.GetRefs("goesaway")
				So(err, ShouldBeNil)
				So(proj, ShouldResemble, exp)
				So(proj, ShouldNotResemble, exp2)
			})
		})
	})
}
