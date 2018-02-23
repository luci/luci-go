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
	"crypto/sha256"
	"io/ioutil"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/milo"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/logdog/common/fetcher"
	logdog_types "go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"
)

// GetMulti retrieves the (possibly cached-by-sha256) Manifest protos.
//
// This will also cache the results into memcache if it does end up fetching it
// from logdog.
func GetMulti(ctx context.Context, links ...*milo.ManifestLink) ([]*milo.Manifest, [][]byte, error) {
	retLock := sync.Mutex{}
	retManifests := make([]*milo.Manifest, len(links))
	retHashes := make([][]byte, len(links))

	err := parallel.RunMulti(ctx, 8, func(mr parallel.MultiRunner) error {
		return mr.RunMulti(func(ch chan<- func() error) {
			for i, link := range links {
				i, link := i, link
				ch <- func() error {
					m, h, err := Get(ctx, link)
					retLock.Lock()
					defer retLock.Unlock()
					retManifests[i] = m
					retHashes[i] = h
					return err
				}
			}
		})
	})

	return retManifests, retHashes, err
}

// Get retrieves the (possibly cached-by-sha256) Manifest proto.
//
// This will also cache the result into memcache if it does end up fetching it
// from logdog.
func Get(ctx context.Context, a *milo.ManifestLink) (*milo.Manifest, []byte, error) {
	if len(a.Sha256) == sha256.Size {
		// cached, mebbeh?
		entry := newSrcManCacheEntry(a.Sha256)
		switch err := datastore.Get(ctx, entry); err {
		case nil:
			ret, err := entry.Read()
			if err == nil {
				return ret, a.Sha256, nil
			}

			logging.WithError(err).Warningf(ctx, "bad srcManCacheEntry(%x), deleting...", a.Sha256)
			if err := datastore.Delete(ctx, entry); err != nil {
				logging.WithError(err).Warningf(ctx, "failed to delete bad cache entry")
			}

		default:
			return nil, nil, errors.Annotate(err, "reading srcManCacheEntry %x", a.Sha256).Tag(transient.Tag).Err()

		case datastore.ErrNoSuchEntity:
		}
	}

	u, err := url.Parse(a.Url)
	if err != nil {
		return nil, nil, errors.Annotate(err, "parsing manifest URL: %q", a.Url).Err()
	}

	var data []byte
	switch u.Scheme {
	case "logdog":
		data, err = logdogGet(ctx, u)
	default:
		err = errors.New("unsupported URL scheme")
	}
	if err != nil {
		return nil, nil, errors.Annotate(err, "fetching from %s", u).Err()
	}

	dataSha := sha256.Sum256(data)
	if len(a.Sha256) == sha256.Size && !bytes.Equal(a.Sha256, dataSha[:]) {
		logging.Warningf(ctx, "mismatched Sha256 for %q (%x, expected %x)", dataSha, a.Sha256)
	}

	entry := newSrcManCacheEntry(dataSha[:])
	entry.Data = data
	ret, err := entry.Read()
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to unmarshal").Err()
	}

	// only write cache if it's good
	if err := datastore.Put(ctx, entry); err != nil {
		logging.WithError(err).Warningf(ctx, "failed to cache manifest %x", dataSha)
	}

	return ret, dataSha[:], nil
}

// logdogGet retrieves the manifest data from a logdog url.
func logdogGet(ctx context.Context, u *url.URL) ([]byte, error) {
	parts := strings.SplitN(u.Path, "/", 3)
	if len(parts) != 3 || parts[0] != "" {
		return nil, errors.Reason("malformed logdog URL: %q", u).Err()
	}
	project := logdog_types.ProjectName(parts[1])
	streamPath := logdog_types.StreamPath(parts[2])

	if err := project.Validate(); err != nil {
		return nil, errors.Annotate(err, "invalid project in URL: %q", project).Err()
	}

	if err := streamPath.Validate(); err != nil {
		return nil, errors.Annotate(err, "invalid streamPath in URL: %q", project).Err()
	}

	client, err := rawpresentation.NewClient(ctx, u.Host)
	if err != nil {
		return nil, err
	}

	stream := client.Stream(project, streamPath)

	data, err := ioutil.ReadAll(stream.Fetcher(ctx, &fetcher.Options{
		RequireCompleteStream: true,
	}).Reader())
	err = errors.Annotate(err, "failed to read stream").Tag(transient.Tag).Err()
	return data, err
}
