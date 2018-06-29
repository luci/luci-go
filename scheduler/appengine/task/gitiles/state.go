// Copyright 2016 The LUCI Authors.
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

package gitiles

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/url"
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"golang.org/x/net/context"
)

var (
	metricTaskGitilesStoredSize = metric.NewInt(
		"luci/scheduler/task/gitiles/stored/size",
		"Size of serialized state in bytes.",
		&types.MetricMetadata{Units: types.Bytes},
		field.String("jobID"),
	)

	metricTaskGitilesStoredRefs = metric.NewInt(
		"luci/scheduler/task/gitiles/stored/refs",
		"Number of refs stored in a serialized state.",
		nil,
		field.String("jobID"),
	)
)

// Reference [DEPRECATED] is used to store the revision of a ref.
type Reference struct {
	// Name is the reference name.
	Name string `gae:",noindex"`

	// Revision is the ref commit.
	Revision string `gae:",noindex"`
}

// Repository is used to store the repository status.
type Repository struct {
	_kind  string         `gae:"$kind,gitiles.Repository"`
	_extra ds.PropertyMap `gae:"-,extra"`

	// ID is "<job ID>:<repository URL>".
	ID string `gae:"$id"`

	// References is the slice of all the tracked refs within repository.
	// DEPRECATED. Used only for reading data from store. Remains here to recover state
	// TODO(tandrii): remove after Nov 30 2017.
	References []Reference `gae:",noindex"`

	// CompressedState stores gzip-compressed proto-serialized list of watched
	// refs with hashes of their tips.
	CompressedState []byte `gae:",noindex"`
}

func repositoryID(jobID string, u *url.URL) string {
	return fmt.Sprintf("%s:%s", jobID, u)
}

func loadState(c context.Context, jobID string, u *url.URL) (map[string]string, error) {
	stored := Repository{ID: repositoryID(jobID, u)}
	err := ds.Get(c, &stored)
	switch {
	case err != nil && err != ds.ErrNoSuchEntity:
		return nil, err

	case len(stored.References) > 0:
		// Load old way of storing refs.
		refTips := make(map[string]string, len(stored.References))
		for _, b := range stored.References {
			refTips[b.Name] = b.Revision
		}
		return refTips, nil

	case len(stored.CompressedState) > 0:
		unGzip, err := gzip.NewReader(bytes.NewBuffer(stored.CompressedState))
		if err != nil {
			return nil, err
		}
		uncompressed, err := ioutil.ReadAll(unGzip)
		if err != nil {
			return nil, err
		}
		if err = unGzip.Close(); err != nil {
			return nil, err
		}

		var state RepositoryState
		if err = proto.Unmarshal(uncompressed, &state); err != nil {
			return nil, err
		}

		heads := map[string]string{}
		for _, space := range state.Spaces {
			for _, child := range space.Children {
				heads[space.Prefix+"/"+child.Suffix] = hex.EncodeToString(child.Sha1)
			}
		}
		return heads, nil

	default:
		return map[string]string{}, nil
	}
}

func saveState(c context.Context, jobID string, u *url.URL, refTips map[string]string) error {
	// There could be many refTips in repos, though most will share some prefix.
	// So we trade CPU to save this efficiently.

	byNamespace := map[string]*RefSpace{}
	for ref, sha1 := range refTips {
		sha1bytes, err := hex.DecodeString(sha1)
		if err != nil {
			return err
		}
		lastSlash := strings.LastIndex(ref, "/")
		ns, suffix := ref[:lastSlash], ref[lastSlash+1:]
		child := &Child{Sha1: sha1bytes, Suffix: suffix}
		if namespace, exists := byNamespace[ns]; exists {
			namespace.Children = append(namespace.Children, child)
		} else {
			byNamespace[ns] = &RefSpace{
				Prefix:   ns,
				Children: []*Child{child},
			}
		}
	}

	spaces := make(sortedSpaces, 0, len(byNamespace))
	for _, space := range byNamespace {
		cs := sortedChildren(space.Children)
		sort.Sort(cs)
		spaces = append(spaces, space)
	}
	sort.Sort(spaces)

	serialized, err := proto.Marshal(&RepositoryState{Spaces: spaces})
	if err != nil {
		return err
	}
	compressed := &bytes.Buffer{}
	w := gzip.NewWriter(compressed)
	if _, err := w.Write(serialized); err != nil {
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}

	metricTaskGitilesStoredRefs.Set(c, int64(len(refTips)), jobID)
	metricTaskGitilesStoredSize.Set(c, int64(compressed.Len()), jobID)
	return transient.Tag.Apply(ds.Put(c, &Repository{
		ID:              repositoryID(jobID, u),
		CompressedState: compressed.Bytes(),
	}))
}

type sortedSpaces []*RefSpace

func (s sortedSpaces) Len() int           { return len(s) }
func (s sortedSpaces) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s sortedSpaces) Less(i, j int) bool { return s[i].Prefix < s[j].Prefix }

type sortedChildren []*Child

func (s sortedChildren) Len() int           { return len(s) }
func (s sortedChildren) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s sortedChildren) Less(i, j int) bool { return s[i].Suffix < s[j].Suffix }
