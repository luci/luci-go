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

package dm

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/duration"

	"go.chromium.org/luci/common/data/text/templateproto"
	"go.chromium.org/luci/common/errors"
)

// IsEmpty returns true if this metadata retry message only contains
// zero-values.
func (q *Quest_Desc_Meta_Retry) IsEmpty() bool {
	return q.Crashed == 0 && q.Expired == 0 && q.Failed == 0 && q.TimedOut == 0
}

func checkPositiveDuration(d *duration.Duration) error {
	dur, err := ptypes.Duration(d)
	if err != nil {
		return err
	}
	if dur < 0 {
		return errors.Reason("duration(%d) < 0", dur).Err()
	}
	return nil
}

// Normalize ensures that all timeouts are >= 0
func (t *Quest_Desc_Meta_Timeouts) Normalize() error {
	if err := checkPositiveDuration(t.Start); err != nil {
		return errors.Annotate(err, "desc.meta.timeouts.start").Err()
	}
	if err := checkPositiveDuration(t.Run); err != nil {
		return errors.Annotate(err, "desc.meta.timeouts.run").Err()
	}
	if err := checkPositiveDuration(t.Stop); err != nil {
		return errors.Annotate(err, "desc.meta.timeouts.stop").Err()
	}
	return nil
}

// IsEmpty returns true if this metadata only contains zero-values.
func (q *Quest_Desc_Meta) IsEmpty() bool {
	return q.AsAccount == "" && q.Retry.IsEmpty()
}

var (
	// QuestIDLength is the number of encoded bytes to use. It removes the
	// single padding character.
	QuestIDLength = base64.URLEncoding.EncodedLen(sha256.Size) - 1
)

// QuestDescPayloadMaxLength is the maximum length (in bytes) of an
// un-normalized Quest payload.
const QuestDescPayloadMaxLength = 256 * 1024

// Normalize returns an error iff the Quest_Desc is invalid.
func (q *Quest_Desc) Normalize() error {
	if q.Meta == nil {
		q.Meta = &Quest_Desc_Meta{
			Retry:    &Quest_Desc_Meta_Retry{},
			Timeouts: &Quest_Desc_Meta_Timeouts{},
		}
	} else {
		if q.Meta.Retry == nil {
			q.Meta.Retry = &Quest_Desc_Meta_Retry{}
		}
		if q.Meta.Timeouts == nil {
			q.Meta.Timeouts = &Quest_Desc_Meta_Timeouts{}
		}
	}
	if err := q.Meta.Timeouts.Normalize(); err != nil {
		return err
	}

	length := len(q.Parameters) + len(q.DistributorParameters)
	if length > QuestDescPayloadMaxLength {
		return fmt.Errorf("quest payload is too large: %d > %d",
			length, QuestDescPayloadMaxLength)
	}
	normed, err := templateproto.NormalizeJSON(q.Parameters, true)
	if err != nil {
		return fmt.Errorf("failed to normalize parameters: %s", err)
	}
	q.Parameters = normed

	normed, err = templateproto.NormalizeJSON(q.DistributorParameters, true)
	if err != nil {
		return fmt.Errorf("failed to normalize distributor parameters: %s", err)
	}
	q.DistributorParameters = normed
	return nil
}

// QuestID computes the DM compatible quest ID for this Quest_Desc. The
// Quest_Desc should already be Normalize()'d.
func (q *Quest_Desc) QuestID() string {
	data, err := proto.Marshal(q)
	if err != nil {
		panic(err)
	}
	h := sha256.Sum256(data)
	return base64.URLEncoding.EncodeToString(h[:])[:QuestIDLength]
}
