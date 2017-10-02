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
	"time"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/git"
)

// Proto converts this User to its protobuf equivalent.
func (u *User) Proto() (ret *git.Commit_User, err error) {
	ret = &git.Commit_User{
		Name:  u.Name,
		Email: u.Email,
	}
	if u.Time != "" {
		var ts time.Time
		if ts, err = u.GetTime(); err != nil {
			err = errors.Annotate(err, "decoding time").Err()
			return
		}
		ret.Time, err = ptypes.TimestampProto(ts)
		err = errors.Annotate(err, "encoding time").Err()
	}
	return
}

// Proto converts this git.Commit to its protobuf equivalent.
func (c *Commit) Proto() (ret *git.Commit, err error) {
	ret = &git.Commit{}

	if ret.Id, err = hex.DecodeString(c.Commit); err != nil {
		err = errors.Annotate(err, "decoding id").Err()
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
	if ret.Author, err = c.Author.Proto(); err != nil {
		err = errors.Annotate(err, "decoding author").Err()
		return
	}
	if ret.Committer, err = c.Committer.Proto(); err != nil {
		err = errors.Annotate(err, "decoding committer").Err()
		return
	}
	ret.Message = c.Message

	// TODO(iannucci): treediff

	return
}

// LogProto takes log data from e.g. Log(), and returns the LUCI standard
// protobuf message form of the log data (suitable for embedding in other proto
// messages).
//
// Errors may occur if the Commits contain bad hashes (i.e. not hexadecimal), or
// undecodable timestamps. This should not occur if the []Commit was produced by
// the Log function in this package.
func LogProto(log []Commit) (ret []*git.Commit, err error) {
	ret = make([]*git.Commit, len(log))
	for i, c := range log {
		if ret[i], err = c.Proto(); err != nil {
			return nil, errors.Annotate(err, "converting commit %d", i).Err()
		}
	}
	return ret, nil
}
