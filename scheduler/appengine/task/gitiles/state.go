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
	"context"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/url"
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/scheduler/appengine/task/gitiles/pb"
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

func legacyRepositoryID(jobID, repo string) (string, error) {
	u, err := url.Parse(repo)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%s", jobID, u), nil
}

func repositoryID(jobID, repo string) (string, error) {
	host, proj, err := gitiles.ParseRepoURL(repo)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{jobID, host, proj}, "\x00 "), nil
}

// loadStateEntry loads Repository instance from datastore.
func loadStateEntry(c context.Context, jobID, repo string) (*Repository, error) {
	id, err := repositoryID(jobID, repo)
	if err != nil {
		return nil, err
	}
	entry := &Repository{ID: id}
	if err := ds.Get(c, entry); err != ds.ErrNoSuchEntity {
		return entry, transient.Tag.Apply(err)
	}

	if entry.ID, err = legacyRepositoryID(jobID, repo); err != nil {
		return nil, err
	}
	if err := ds.Get(c, entry); err == ds.ErrNoSuchEntity {
		return nil, err
	} else {
		return entry, transient.Tag.Apply(err)
	}
}

func saveStateEntry(c context.Context, jobID, repo string, compressedBytes []byte) error {
	id, err := repositoryID(jobID, repo)
	if err != nil {
		return err
	}
	legacyID, err := legacyRepositoryID(jobID, repo)
	if err != nil {
		return err
	}
	entry := Repository{CompressedState: compressedBytes}
	return transient.Tag.Apply(ds.RunInTransaction(c, func(c context.Context) error {
		entry.ID = id
		if err := ds.Put(c, &entry); err != nil {
			return err
		}
		entry.ID = legacyID
		return ds.Put(c, &entry)
	}, &ds.TransactionOptions{XG: true}))
}

func loadState(c context.Context, jobID, repo string) (map[string]string, error) {
	switch stored, err := loadStateEntry(c, jobID, repo); {
	case err == ds.ErrNoSuchEntity:
		return map[string]string{}, nil
	case err != nil:
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

		var state pb.RepositoryState
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

func saveState(c context.Context, jobID, repo string, refTips map[string]string) error {
	// There could be many refTips in repos, though most will share some prefix.
	// So we trade CPU to save this efficiently.

	byNamespace := map[string]*pb.RefSpace{}
	for ref, sha1 := range refTips {
		sha1bytes, err := hex.DecodeString(sha1)
		if err != nil {
			return err
		}
		lastSlash := strings.LastIndex(ref, "/")
		ns, suffix := ref[:lastSlash], ref[lastSlash+1:]
		child := &pb.Child{Sha1: sha1bytes, Suffix: suffix}
		if namespace, exists := byNamespace[ns]; exists {
			namespace.Children = append(namespace.Children, child)
		} else {
			byNamespace[ns] = &pb.RefSpace{
				Prefix:   ns,
				Children: []*pb.Child{child},
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

	serialized, err := proto.Marshal(&pb.RepositoryState{Spaces: spaces})
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
	return saveStateEntry(c, jobID, repo, compressed.Bytes())
}

type sortedSpaces []*pb.RefSpace

func (s sortedSpaces) Len() int           { return len(s) }
func (s sortedSpaces) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s sortedSpaces) Less(i, j int) bool { return s[i].Prefix < s[j].Prefix }

type sortedChildren []*pb.Child

func (s sortedChildren) Len() int           { return len(s) }
func (s sortedChildren) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s sortedChildren) Less(i, j int) bool { return s[i].Suffix < s[j].Suffix }
