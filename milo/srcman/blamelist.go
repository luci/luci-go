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
	"sync"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/memcache"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/base128"
	"go.chromium.org/luci/common/errors"
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
	return ret, nil
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

func manifestDiffMCItem(c context.Context, hashA, hashB []byte, options *DiffOptions) (memcache.Item, bool) {
	if len(hashA) == sha256.Size && len(hashB) == sha256.Size {
		return memcache.NewItem(c, fmt.Sprintf("DiffWithBlamelist(%s,%s,%s)",
			options,
			base128.EncodeToString(hashA),
			base128.EncodeToString(hashB))), true
	}
	return nil, false
}

// DiffWithBlamelist retrieves the manifests, computes a diff, then adds git
// history logs for git checkouts with a status of "DIFF".
//
// These operations are cached in memcache (not datastore).
func DiffWithBlamelist(c context.Context, a, b *milo.ManifestLink, options *DiffOptions) (*milo.ManifestDiff, error) {
	if mi, ok := manifestDiffMCItem(c, a.Sha256, b.Sha256, options); ok {
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

	var retLock sync.Mutex
	ret := mans[0].Diff(mans[1])

	if options.WithHistory {
		t, err := auth.GetRPCTransport(c, auth.AsUser)
		if err != nil {
			return nil, errors.Annotate(err, "getting authenticated transport").Err()
		}
		g := gitiles.Client{Client: &http.Client{Transport: t}}

		err = parallel.RunMulti(c, 8, func(mr parallel.MultiRunner) error {
			return mr.RunMulti(func(ch chan<- func() error) {
				for dirname, dir := range ret.Directories {
					if dir.GetGitCheckout().Overall == milo.ManifestDiff_DIFF {
						dirname, dir := dirname, dir

						ch <- func() error {
							treeish := fmt.Sprintf("%s..%s",
								ret.Old.Directories[dirname].GitCheckout.Revision,
								ret.New.Directories[dirname].GitCheckout.Revision,
							)
							log, err := g.Log(c, dir.GitCheckout.RepoUrl, treeish, 100, options.WithFiles)
							if err != nil {
								return errors.Annotate(err, "fetching log").Err()
							}

							pLog, err := gitiles.ProtoizeLog(log)
							if err != nil {
								return errors.Annotate(err, "protoizing log").Err()
							}

							retLock.Lock()
							defer retLock.Unlock()
							ret.Directories[dirname].GitCheckout.History = pLog
							return nil
						}
					}
				}
			})
		})

		return nil, errors.Annotate(err, "getting histories").Err()
	}

	if mi, ok := manifestDiffMCItem(c, hashes[0], hashes[1], options); ok {
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
