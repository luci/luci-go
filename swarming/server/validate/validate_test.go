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

package validate

import (
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/directoryocclusion"
)

func TestDimensionKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		dim string
		err any
	}{
		{"good", nil},
		{strings.Repeat("a", maxDimensionKeyLen), nil},
		{"", "cannot be empty"},
		{strings.Repeat("a", maxDimensionKeyLen+1), "should be no longer"},
		{"bad key", "should match"},
	}

	for _, cs := range cases {
		t.Run(cs.dim, func(t *testing.T) {
			assert.That(t, DimensionKey(cs.dim), should.ErrLike(cs.err))
		})
	}
}

func TestDimensionValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		key string
		dim string
		err any
	}{
		{"dim", "good value", nil},
		{"dim", "good:value", nil},
		{"dim", strings.Repeat("a", maxDimensionValLen), nil},
		{"dim", "", "cannot be empty"},
		{"dim", strings.Repeat("a", maxDimensionValLen+1), "should be no longer"},
		{"dim", " bad value", "no leading or trailing spaces"},
		{"dim", "bad value ", "no leading or trailing spaces"},
		{"id", "bad:value", `bot ID is not allowed to contain ":"`},
		{"pool", "bad:value", `pool ID is not allowed to contain ":"`},
	}

	for _, cs := range cases {
		t.Run(cs.dim, func(t *testing.T) {
			assert.That(t, DimensionValue(cs.key, cs.dim), should.ErrLike(cs.err))
		})
	}
}

func TestSessionID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		val string
		err any
	}{
		{"good-value_/09", nil},
		{strings.Repeat("a", 50), nil},
		{"", "should match"},
		{strings.Repeat("a", 51), "should match"},
		{"BAD", "should match"},
	}

	for _, cs := range cases {
		t.Run(cs.val, func(t *testing.T) {
			assert.That(t, SessionID(cs.val), should.ErrLike(cs.err))
		})
	}
}

func TestBotRequestUUID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		val string
		err any
	}{
		{"good-value_/09AZ", nil},
		{"", nil},
		{strings.Repeat("a", 50), nil},
		{strings.Repeat("a", 51), "should match"},
		{"   ", "should match"},
	}

	for _, cs := range cases {
		t.Run(cs.val, func(t *testing.T) {
			assert.That(t, BotRequestUUID(cs.val), should.ErrLike(cs.err))
		})
	}
}

func TestTag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tag string
		err any
	}{
		// OK
		{"k:v", nil},
		{"", "tag must be in key:value form"},
		{fmt.Sprintf("%s:v", strings.Repeat("k", maxDimensionKeyLen)), nil},
		{fmt.Sprintf("k:%s", strings.Repeat("v", maxDimensionValLen)), nil},
		{"k:", nil},
		{"k:v1:subv2", nil},
		// key
		{":v", "the key cannot be empty"},
		{fmt.Sprintf("%s:v", strings.Repeat("k", maxDimensionKeyLen+1)),
			"should be no longer"},
		// value
		{"k: v", "no leading or trailing spaces"},
		{"k:v ", "no leading or trailing spaces"},
		{fmt.Sprintf("k:%s", strings.Repeat("v", maxDimensionValLen+1)),
			"should be no longer"},
		// reserved
		{"swarming.terminate:1", "reserved"},
	}

	for _, cs := range cases {
		t.Run(cs.tag, func(t *testing.T) {
			assert.That(t, Tag(cs.tag), should.ErrLike(cs.err))
		})
	}
}

func TestPriority(t *testing.T) {
	t.Parallel()
	cases := []struct {
		p   int32
		err any
	}{
		{40, nil},
		{1, nil},
		{255, nil},
		{0, "must be between 1 and 255"},
		{-1, "must be between 1 and 255"},
		{256, "must be between 1 and 255"},
	}

	for _, cs := range cases {
		t.Run(fmt.Sprint(cs.p), func(t *testing.T) {
			assert.That(t, Priority(cs.p), should.ErrLike(cs.err))
		})
	}
}

func TestServiceAccount(t *testing.T) {
	t.Parallel()
	cases := []struct {
		sa  string
		err any
	}{
		{"sa@service-accounts.com", nil},
		{strings.Repeat("l", maxServiceAccountLength+1), "too long"},
		{"", "invalid"},
		{"invalid", "invalid"},
	}

	for _, cs := range cases {
		t.Run(cs.sa, func(t *testing.T) {
			assert.That(t, ServiceAccount(cs.sa), should.ErrLike(cs.err))
		})
	}
}

func TestBotPingTolerance(t *testing.T) {
	t.Parallel()
	cases := []struct {
		bpt int64
		err any
	}{
		{300, nil},
		{60, nil},
		{1200, nil},
		{-1, "must be between 60 and 1200"},
		{1201, "must be between 60 and 1200"},
	}

	for _, cs := range cases {
		t.Run(fmt.Sprint(cs.bpt), func(t *testing.T) {
			assert.That(t, BotPingTolerance(cs.bpt), should.ErrLike(cs.err))
		})
	}
}

func TestSecureURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url string
		err any
	}{
		{"https://example.com", nil},
		{"https://user:pass@bar.com", nil},
		{"http://127.0.0.1", nil},
		{"https://localhost", nil},
		{"http://localhost/", nil},
		{"http://localhost/yo", nil},
		{"http://localhost:1", nil},
		{"https://localhost:1/yo", nil},
		{"http://example.com", "not secure"},
		{"ftp://example.com", "not secure"},
		{"ftp://localhost", "not secure"},
		{"invalid", "invalid"},
		{"http://#yo", "invalid"},
		{"http://", "invalid"},
		{"http://localhost:pwd@evil.com", "not secure"},
	}
	for _, cs := range cases {
		t.Run(cs.url, func(t *testing.T) {
			assert.That(t, SecureURL(cs.url), should.ErrLike(cs.err))
		})
	}
}

func TestPubSubTopicName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		topicName string
		project   string
		topic     string
		err       any
	}{
		{"empty", "", "", "", nil},
		{"valid", "projects/project/topics/topic", "project", "topic", nil},
		{"internal_project", "projects/google.com:proj/topics/topic", "google.com:proj", "topic", nil},
		{"too_long", strings.Repeat("l", maxPubsubTopicLength+1), "", "", "too long"},
		{"name_invalid", "invalid", "", "", "not match"},
		{"project_invalid", "projects/1invalid/topics/topic", "", "", "not match"},
		{"topic_invalid", "projects/project/topics/1invalid", "", "", "not match"},
		{"topic_with_goog_prefix", "projects/project/topics/googtopic", "", "", "shouldn't begin with the string goog"},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			project, topic, err := PubSubTopicName(cs.topicName)
			assert.That(t, project, should.Equal(cs.project))
			assert.That(t, topic, should.Equal(cs.topic))
			assert.That(t, err, should.ErrLike(cs.err))
		})
	}
}

func TestPath(t *testing.T) {
	t.Parallel()
	maxLen := 255
	cases := []struct {
		name         string
		path         string
		allowWinPath bool
		err          any
	}{
		{"empty", "", false, "cannot be empty"},
		{"too_long", strings.Repeat("a", maxLen+1), false, "too long"},
		{"with_double_backslashes", "a\\b", false, `cannot contain "\\".`},
		{"with_double_backslashes_but_allow", "a\\b", true, nil},
		{"with_leading_slash", "/a/b", false, `cannot start with "/"`},
		{"not_normalized_dot", "./a/b", false, "is not normalized"},
		{"not_normalized_double_dots", "a/../b", false, "is not normalized"},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			err := Path(cs.path, maxLen, cs.allowWinPath)
			assert.That(t, err, should.ErrLike(cs.err))
		})
	}
}

func TestCaches(t *testing.T) {
	t.Parallel()

	ftt.Run("Caches", t, func(t *ftt.Test) {
		t.Run("too many caches", func(t *ftt.Test) {
			cache := &apipb.CacheEntry{
				Name: "name",
				Path: "path",
			}
			var caches []*apipb.CacheEntry
			for range maxCacheCount + 1 {
				caches = append(caches, cache)
			}
			_, err := Caches(caches, "task_cache")
			assert.That(t, err.AsError(), should.ErrLike(fmt.Sprintf("can have up to %d caches", maxCacheCount)))
		})

		t.Run("name", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				cases := []struct {
					tn   string
					name string
					err  any
				}{
					{"empty", "", "required"},
					{"too_long", strings.Repeat("a", maxCacheNameLength+1), "too long"},
					{"invalid", "INVALID", "should match"},
				}
				for _, cs := range cases {
					t.Run(cs.tn, func(t *ftt.Test) {
						caches := []*apipb.CacheEntry{
							{
								Name: cs.name,
								Path: "path",
							},
						}
						_, err := Caches(caches, "task_cache")
						assert.Loosely(t, err.AsError(), should.ErrLike(cs.err))
					})
				}
			})
			t.Run("duplicates", func(t *ftt.Test) {
				caches := []*apipb.CacheEntry{
					{
						Name: "name",
						Path: "path",
					},
					{
						Name: "name",
						Path: "path",
					},
				}
				_, err := Caches(caches, "task_cache")
				assert.That(t, err.AsError(), should.ErrLike("same cache name cannot be specified twice"))
			})
		})
		t.Run("path", func(t *ftt.Test) {
			t.Run("duplicates", func(t *ftt.Test) {
				caches := []*apipb.CacheEntry{
					{
						Name: "name1",
						Path: "a/b",
					},
					{
						Name: "name2",
						Path: "a/b",
					},
				}
				_, err := Caches(caches, "task_cache")
				assert.That(t, err.AsError(), should.ErrLike(`"a/b": directory has conflicting owners: task_cache:name1[] and task_cache:name2[]`))
			})
		})
	})
}

func TestCIPDServer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		tn     string
		server string
		err    any
	}{
		{"empty", "", "required"},
		{"too_long", strings.Repeat("a", maxCIPDServerLength+1), "too long"},
	}
	for _, cs := range cases {
		t.Run(cs.tn, func(t *testing.T) {
			assert.That(t, CIPDServer(cs.server), should.ErrLike(cs.err))
		})
	}
}

func TestCIPDPackages(t *testing.T) {
	t.Parallel()

	ftt.Run("CIPDPackages", t, func(t *ftt.Test) {
		t.Run("too_many", func(t *ftt.Test) {
			var pkgs []*apipb.CipdPackage
			for range maxCIPDPackageCount + 1 {
				pkgs = append(pkgs, &apipb.CipdPackage{})
			}
			err := CIPDPackages(pkgs, false, directoryocclusion.NewChecker(""), "task_cipd_packages")
			assert.That(t, err.AsError(), should.ErrLike("can have up to 64 packages"))
		})
		t.Run("duplicate", func(t *ftt.Test) {
			pkgs := []*apipb.CipdPackage{
				{
					PackageName: "some/pkg",
					Version:     "version1",
					Path:        "a/b",
				},
				{
					PackageName: "some/pkg",
					Version:     "version2",
					Path:        "a/b",
				},
			}
			err := CIPDPackages(pkgs, false, directoryocclusion.NewChecker(""), "task_cipd_packages")
			assert.That(t, err.AsError(), should.ErrLike("specified more than once"))
		})
		t.Run("no_path", func(t *ftt.Test) {
			pkgs := []*apipb.CipdPackage{
				{
					PackageName: "some/pkg",
					Version:     "version",
				},
			}
			err := CIPDPackages(pkgs, false, directoryocclusion.NewChecker(""), "task_cipd_packages")
			assert.That(t, err.AsError(), should.ErrLike("path: cannot be empty"))
		})
		t.Run("cache_path", func(t *ftt.Test) {
			pkgs := []*apipb.CipdPackage{
				{
					PackageName: "some/pkg",
					Version:     "version1",
					Path:        "a/b",
				},
			}
			doc := directoryocclusion.NewChecker("")
			doc.Add("a/b", "task_cache:name1", "")
			err := CIPDPackages(pkgs, false, doc, "task_cipd_packages")
			assert.That(t, err.AsError(), should.ErrLike(`"a/b": directory has conflicting owners: task_cache:name1[] and task_cipd_packages[some/pkg:version1]`))
		})
		t.Run("require_pinned_verison", func(t *ftt.Test) {
			t.Run("fail", func(t *ftt.Test) {
				pkgs := []*apipb.CipdPackage{
					{
						PackageName: "some/pkg",
						Version:     "version1",
						Path:        "a/b",
					},
				}
				err := CIPDPackages(pkgs, true, directoryocclusion.NewChecker(""), "task_cipd_packages")
				assert.That(t, err.AsError(), should.ErrLike("cannot have unpinned packages"))
			})
			t.Run("pass_tag", func(t *ftt.Test) {
				pkgs := []*apipb.CipdPackage{
					{
						PackageName: "some/pkg",
						Version:     "good:tag",
						Path:        "a/b",
					},
				}
				err := CIPDPackages(pkgs, true, directoryocclusion.NewChecker(""), "task_cipd_packages")
				assert.NoErr(t, err.AsError())
			})
			t.Run("pass_hash", func(t *ftt.Test) {
				pkgs := []*apipb.CipdPackage{
					{
						PackageName: "some/pkg",
						Version:     "B7r75joOfFfFcq7fHCKAIrU34oeFAT174Bf8eHMajMUC",
						Path:        "a/b",
					},
				}
				err := CIPDPackages(pkgs, true, directoryocclusion.NewChecker(""), "task_cipd_packages")
				assert.NoErr(t, err.AsError())
			})
		})
	})
}

func TestEnvVar(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ev   string
		err  any
	}{
		{"empty", "", "required"},
		{"too_long", strings.Repeat("a", maxEnvVarLength+1), "too long"},
		{"invalid", "1", "should match"},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			err := EnvVar(cs.ev)
			assert.That(t, err, should.ErrLike(cs.err))
		})
	}
}

func TestBotDimensions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		dims map[string][]string
		errs []string
	}{
		{
			"ok",
			map[string][]string{
				"id":  {"something"},
				"key": {"a", "b", "c"},
			},
			nil,
		},
		{
			"bad key",
			map[string][]string{
				"id":    {"something"},
				"  bad": {"a"},
			},
			[]string{"the key should match"},
		},
		{
			"empty vals",
			map[string][]string{
				"id":  {"something"},
				"key": nil,
			},
			[]string{"should not be empty"},
		},
		{
			"bad val",
			map[string][]string{
				"id":  {"something"},
				"key": {"  bad"},
			},
			[]string{"should have no leading or trailing spaces"},
		},
		{
			"dup val",
			map[string][]string{
				"id":  {"something"},
				"key": {"a", "a"},
			},
			[]string{"duplicate value"},
		},
		{
			"multiple ids",
			map[string][]string{
				"id": {"something", "else"},
			},
			[]string{"must have only one value"},
		},
		{
			"missing id",
			map[string][]string{
				"key": {"a", "b"},
			},
			[]string{"a value is missing"},
		},
		{
			"many errors",
			map[string][]string{
				"id": {"something"},
				"a":  {},
				"b":  {},
				"c":  {},
			},
			[]string{`key "a"`, `key "b"`, `key "c"`},
		},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			errs := BotDimensions(cs.dims)
			assert.Loosely(t, errs, should.HaveLength(len(cs.errs)))
			for i, err := range errs {
				assert.That(t, err, should.ErrLike(cs.errs[i]))
			}
		})
	}
}

func TestTrimLen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in  string
		out string
	}{
		{"", ""},
		{"123456", "123456"},
		{"12345", "12345"},
		{"1234567", "123..."},
	}
	for _, cs := range cases {
		assert.That(t, trimLen(cs.in, 6), should.Equal(cs.out))
	}
}
