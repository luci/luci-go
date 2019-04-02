// Copyright 2019 The LUCI Authors.
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

package protoutil

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/data/strpair"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// StringPairs converts a strpair.Map to a slice of StringPair messages.
func StringPairs(m strpair.Map) []*pb.StringPair {
	ret := make([]*pb.StringPair, 0, len(m))
	for k, vs := range m {
		for _, v := range vs {
			ret = append(ret, &pb.StringPair{Key: k, Value: v})
		}
	}
	SortStringPairs(ret)
	return ret
}

// SortStringPairs sorts string pairs.
func SortStringPairs(pairs []*pb.StringPair) {
	sort.Slice(pairs, func(i, j int) bool {
		switch {
		case pairs[i].Key < pairs[j].Key:
			return true
		case pairs[i].Key > pairs[j].Key:
			return false
		default:
			return pairs[i].Value < pairs[j].Value
		}
	})
}

// BuildSets returns all of the buildsets of the build.
func BuildSets(b *pb.Build) []string {
	var result []string
	for _, tag := range b.Tags {
		if tag.Key == "buildset" {
			result = append(result, tag.Value)
		}
	}
	return result
}

// GerritBuildSet returns a buildset representation of c,
// e.g. "patch/gerrit/chromium-review.googlesource.com/677784/5"
func GerritBuildSet(c *pb.GerritChange) string {
	return fmt.Sprintf("patch/gerrit/%s/%d/%d", c.Host, c.Change, c.Patchset)
}

// GerritChangeURL returns URL of the change.
func GerritChangeURL(c *pb.GerritChange) string {
	return fmt.Sprintf("https://%s/c/%d/%d", c.Host, c.Change, c.Patchset)
}

// GitilesBuildSet returns a buildset representation of c.
// e.g. "commit/gitiles/chromium.googlesource.com/infra/luci/luci-go/+/b7a757f457487cd5cfe2dae83f65c5bc10e288b7"
func GitilesBuildSet(c *pb.GitilesCommit) string {
	return fmt.Sprintf("commit/gitiles/%s/%s/+/%s", c.Host, c.Project, c.Id)
}

// GitilesRepoURL returns the URL for the gitiles repo.
// e.g. "https://chromium.googlesource.com/chromium/src"
func GitilesRepoURL(c *pb.GitilesCommit) string {
	return fmt.Sprintf("https://%s/%s", c.Host, c.Project)
}

// GitilesCommitURL returns the URL for the gitiles commit.
// e.g. "https://chromium.googlesource.com/chromium/src/+/b7a757f457487cd5cfe2dae83f65c5bc10e288b7"
func GitilesCommitURL(c *pb.GitilesCommit) string {
	return fmt.Sprintf("%s/+/%s", GitilesRepoURL(c), c.Id)
}

// ParseBuildSet tries to parse buildset as one of the known formats.
// May return *pb.GerritChange, *pb.GitilesCommit
// or nil.
func ParseBuildSet(buildSet string) proto.Message {
	// fmt.Sscanf cannot be used for this parsing because
	//   var a, b string
	//   fmt.Scanf("a/b", "%s/%s", &a, &b)
	//   a == "a/b", b == ""

	p := strings.Split(buildSet, "/")
	for _, c := range p {
		if c == "" {
			return nil
		}
	}
	n := len(p)
	switch {
	case n == 5 && p[0] == "patch" && p[1] == "gerrit":
		gerrit := &pb.GerritChange{
			Host: p[2],
		}
		var err error
		if gerrit.Change, err = strconv.ParseInt(p[3], 10, 64); err != nil {
			return nil
		}
		if gerrit.Patchset, err = strconv.ParseInt(p[4], 10, 64); err != nil {
			return nil
		}
		return gerrit

	case n >= 5 && p[0] == "commit" && p[1] == "gitiles":
		if p[n-2] != "+" || !looksLikeSha1(p[n-1]) {
			return nil
		}
		return &pb.GitilesCommit{
			Host:    p[2],
			Project: strings.Join(p[3:n-2], "/"), // exclude plus
			Id:      p[n-1],
		}

	default:
		return nil
	}
}

func looksLikeSha1(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		switch {
		case '0' <= c && c <= '9':
		case 'a' <= c && c <= 'f':
		default:
			return false
		}
	}
	return true
}
