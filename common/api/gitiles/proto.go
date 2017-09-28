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

package gitiles

import (
	"encoding/hex"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/google"
)

// ToProto converts this gitiles.User to its protobuf equivalent.
func (u *User) ToProto() (ret *gitiles.User, err error) {
	ret = &gitiles.User{
		Name:  u.Name,
		Email: u.Email,
	}
	ts, err := u.GetTime()
	if err != nil {
		err = errors.Annotate(err, "decoding time").Err()
		return
	}
	ret.Time = google.NewTimestamp(ts)
	return
}

// ToProto converts this gitiles.TreeDiff to its protobuf equivalent.
func (t *TreeDiff) ToProto() (ret *gitiles.LogCommit_TreeDiff, err error) {
	ret = &gitiles.LogCommit_TreeDiff{
		OldPath: t.OldPath,
		OldMode: t.OldMode,
		NewPath: t.NewPath,
		NewMode: t.NewMode,
	}

	if val, ok := gitiles.LogCommit_TreeDiff_ChangeType_value[t.Type]; ok {
		ret.Type = gitiles.LogCommit_TreeDiff_ChangeType(val)
	} else {
		err = errors.New("bad change type")
		return
	}

	if ret.OldId, err = hex.DecodeString(t.OldID); err != nil {
		err = errors.Annotate(err, "decoding OldID").Err()
		return
	}

	if ret.NewId, err = hex.DecodeString(t.NewID); err != nil {
		err = errors.Annotate(err, "decoding NewID").Err()
		return
	}

	return
}

// ToProto converts this gitiles.Commit to its protobuf equivalent.
func (c *Commit) ToProto() (ret *gitiles.LogCommit, err error) {
	ret = &gitiles.LogCommit{}

	if ret.Commit, err = hex.DecodeString(c.Commit); err != nil {
		err = errors.Annotate(err, "decoding commit").Err()
		return
	}
	if ret.Tree, err = hex.DecodeString(c.Tree); err != nil {
		err = errors.Annotate(err, "decoding tree").Err()
		return
	}
	if len(c.Parents) > 0 {
		ret.Parents = make([][]byte, len(c.Parents))
		for i, p := range c.Parents {
			if ret.Parents[i], err = hex.DecodeString(p); err != nil {
				err = errors.Annotate(err, "decoding parent %d", i).Err()
				return
			}
		}
	}
	if ret.Author, err = c.Author.ToProto(); err != nil {
		err = errors.Annotate(err, "decoding author").Err()
		return
	}
	if ret.Committer, err = c.Committer.ToProto(); err != nil {
		err = errors.Annotate(err, "decoding committer").Err()
		return
	}
	ret.Message = c.Message

	if len(c.TreeDiff) > 0 {
		ret.TreeDiff = make([]*gitiles.LogCommit_TreeDiff, len(c.TreeDiff))
		for i, d := range c.TreeDiff {
			if ret.TreeDiff[i], err = d.ToProto(); err != nil {
				err = errors.Annotate(err, "decoding treediff %d", i).Err()
				return
			}
		}
	}

	return
}

// ProtoizeLog takes log data from e.g. Log(), and returns the LUCI standard
// protobuf message form of the log data (suitable for embedding in other proto
// messages).
//
// Errors may occur if the Commits contain bad hashes (i.e. not hexadecimal), or
// undecodable timestamps. This should not occur if the []Commit was produced by
// the Log function in this package.
func ProtoizeLog(log []Commit) (ret []*gitiles.LogCommit, err error) {
	ret = make([]*gitiles.LogCommit, len(log))
	for i, c := range log {
		if ret[i], err = c.ToProto(); err != nil {
			return nil, errors.Annotate(err, "converting commit %d", i).Err()
		}
	}
	return ret, nil
}
