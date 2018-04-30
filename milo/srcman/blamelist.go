// Copyright 2017 The LUCI Authors.
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

package srcman

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"strings"
	"sync/atomic"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/base128"
	"go.chromium.org/luci/common/logging"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/milo/git"
)

// DiffOptions allows the caller of DiffWithBlamelist to opt-in to including git
// history. If history is opted for, the caller may also opt to include
// tree-diff information with WithFiles.
type DiffOptions struct {
	WithHistory bool
	WithFiles   bool
}

func (d *DiffOptions) String() string {
	ret := ""
	if d.WithHistory {
		ret += "H"
	}
	if d.WithFiles {
		ret += "F"
	}
	return ret
}

func decodeMD(data []byte) (*milo.ManifestDiff, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	newData, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	ret := &milo.ManifestDiff{}
	if err := proto.Unmarshal(newData, ret); err != nil {
		return nil, err
	}
	return ret, r.Close()
}

func encodeMD(d *milo.ManifestDiff) ([]byte, error) {
	raw, err := proto.Marshal(d)
	if err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	w := gzip.NewWriter(buf)
	if _, err = w.Write(raw); err != nil {
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func mdCacheItem(c context.Context, hashA, hashB []byte, options *DiffOptions) memcache.Item {
	return memcache.NewItem(c, fmt.Sprintf("DiffWithBlamelist(%s,%s,%s)",
		options,
		base128.EncodeToString(hashA),
		base128.EncodeToString(hashB)))
}

// PopulateHistory attempts to populate the git histories for any DIFF-status
// GitCheckouts in the ManifestDiff.
//
// This is only able to populate histories from googlesource.com-based git
// repos.
//
// Any errors fetching the histories will be logged, but the errors will not be
// returned.
//
// This function returns true iff all git histories that COULD be filled in,
// were actually successfully filled in (and thus, should the resulting diff
// object be cached).
func PopulateHistory(c context.Context, diff *milo.ManifestDiff, withFiles bool) (ok bool) {
	logging.Infof(c, "populating git history in ManifestDiff")

	// hadErrors will be set to 1 if there were any errors.
	hadErrors := uint32(0)

	// always returns RunMulti's error, which is nil, because process returns
	// nil.
	_ = parallel.RunMulti(c, 8, func(mr parallel.MultiRunner) error {
		return mr.RunMulti(func(ch chan<- func() error) {
			for dirname, dir := range diff.Directories {
				dirname := dirname

				gitCheckout := dir.GetGitCheckout()
				if gitCheckout == nil {
					continue
				}
				if gitCheckout.Revision != milo.ManifestDiff_DIFF {
					continue
				}

				project, host, err := gitiles.ParseRepoURL(gitCheckout.RepoUrl)
				if err != nil {
					logging.WithError(err).Warningf(c, "could not parse RepoURL %q for dir %q", gitCheckout.RepoUrl, dirname)
					continue
				}

				if !strings.HasSuffix(host, ".googlesource.com") {
					logging.WithError(err).Warningf(c, "unsupported git host %q for dir %q", gitCheckout.RepoUrl, dirname)
					continue
				}

				ch <- func() error {
					client, err := git.Client(c, host)
					if err != nil {
						logging.WithError(err).Warningf(c, "creating gitiles client")
						atomic.StoreUint32(&hadErrors, 1)
						return nil
					}

					res, err := client.Log(c, &gitilespb.LogRequest{
						Project:  project,
						Ancestor: diff.Old.Directories[dirname].GitCheckout.Revision,
						Treeish:  diff.New.Directories[dirname].GitCheckout.Revision,
						TreeDiff: withFiles,
						PageSize: 100,
					})
					if err != nil {
						logging.WithError(err).Warningf(c, "fetching log - %q", dirname)
						atomic.StoreUint32(&hadErrors, 1)
						return nil
					}
					gitCheckout.History = res.Log
					return nil
				}
			}
		})
	})

	return hadErrors == 0
}

// DiffWithBlamelist retrieves the manifests, computes a diff, then optionally
// adds git history logs for git checkouts with a status of "DIFF".
//
// These operations are cached in memcache (not datastore).
func DiffWithBlamelist(c context.Context, a, b *milo.ManifestLink, options *DiffOptions) (*milo.ManifestDiff, error) {
	if len(a.Sha256) == sha256.Size && len(a.Sha256) == sha256.Size {
		mi := mdCacheItem(c, a.Sha256, b.Sha256, options)
		if err := memcache.Get(c, mi); err != nil {
			ret, err := decodeMD(mi.Value())
			if err != nil {
				logging.WithError(err).Warningf(c, "error decoding cached diff")
			} else {
				return ret, nil
			}
		}
	}

	// need to fetch these for real.
	mans, hashes, err := GetMulti(c, a, b)
	if err != nil {
		return nil, err
	}

	ret := mans[0].Diff(mans[1])
	shouldCache := true

	if options.WithHistory {
		shouldCache = PopulateHistory(c, ret, options.WithFiles)
	}

	if shouldCache {
		mi := mdCacheItem(c, hashes[0], hashes[1], options)
		data, err := encodeMD(ret)
		if err != nil {
			logging.WithError(err).Warningf(c, "error encoding diff cache value")
		} else {
			if err := memcache.Set(c, mi.SetValue(data)); err != nil {
				logging.WithError(err).Warningf(c, "error writing diff cache value")
			}
		}
	}
	return ret, nil
}
