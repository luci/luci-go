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
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/base128"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/auth"
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

// HACK(iannucci,hinoka,tandrii) - Until Milo properly supports ACLs for
// blamelists, we have a hack; if the git repo being diff'd is in this list, use
// `auth.AsSelf`. Otherwise use `auth.Anonymous`.
//
// The reason to do this is that we currently do blamelist calculation in the
// backend, so we can't accurately determine if the requesting user has access
// to these repos or not. For now, we use this whitelist to indicate domains
// that we know have full public read-access so that we can use milo's
// credentials (instead of anonymous) in order to avoid hitting gitiles'
// anonymous quota limits.
var whitelistDomains = stringset.NewFromSlice(
	"chromium.googlesource.com",
)

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

	var noauthClient *gitiles.Client
	if t, err := auth.GetRPCTransport(c, auth.NoAuth); err != nil {
		logging.WithError(err).Errorf(c, "getting NoAuth transport")
	} else {
		noauthClient = &gitiles.Client{Client: &http.Client{Transport: t}}
	}

	var selfClient *gitiles.Client
	if t, err := auth.GetRPCTransport(c, auth.AsSelf); err != nil {
		logging.WithError(err).Errorf(c, "getting AsSelf transport")
	} else {
		selfClient = &gitiles.Client{Client: &http.Client{Transport: t}}
	}

	if selfClient == nil && noauthClient == nil {
		logging.Warningf(c, "could not build any gitiles.Clients, aborting")
		return
	}

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
				if gitCheckout.Overall != milo.ManifestDiff_DIFF {
					continue
				}

				u, err := url.Parse(gitCheckout.RepoUrl)
				if err != nil {
					logging.WithError(err).Warningf(c, "could not parse RepoUrl %q / %q", dirname, gitCheckout.RepoUrl)
					continue
				}

				if !strings.HasSuffix(u.Host, ".googlesource.com") {
					logging.WithError(err).Warningf(c, "unsupported git host %q / %q", dirname, gitCheckout.RepoUrl)
					continue
				}

				ch <- func() error {
					client := noauthClient
					if whitelistDomains.Has(u.Host) && selfClient != nil {
						client = selfClient
					}

					opts := []gitiles.LogOption{gitiles.Limit(100)}
					if withFiles {
						opts = append(opts, gitiles.WithTreeDiff)
					}

					treeish := fmt.Sprintf(
						"%s..%s",
						diff.Old.Directories[dirname].GitCheckout.Revision,
						diff.New.Directories[dirname].GitCheckout.Revision,
					)
					log, err := client.Log(c, gitCheckout.RepoUrl, treeish, opts...)
					if err != nil {
						logging.WithError(err).Warningf(c, "fetching log - %q", dirname)
						atomic.StoreUint32(&hadErrors, 1)
						return nil
					}

					gitCheckout.History, err = gitiles.LogProto(log)
					if err != nil {
						logging.WithError(err).Warningf(c, "protoizing log - %q", dirname)
						atomic.StoreUint32(&hadErrors, 1)
						return nil
					}

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
