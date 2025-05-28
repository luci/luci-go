// Copyright 2020 The LUCI Authors.
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

// Package botdata implements parsing and generation logic for BotData.
package botdata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/cv/internal/gerrit"
)

// ActionType describes the type of the action taken by CV against a Gerrit
// Change.
type ActionType string

const (
	// Start indicates that LUCI CV has started a Run for a Gerrit Change.
	Start ActionType = "start"
	// Cancel indicates that LUCI CV has cancelled a Run for Gerrit Change.
	Cancel = "cancel"
)

const (
	// botDataPrefix is a string that prepends to the BotData message.
	botDataPrefix = "Bot data: "
	// botDataSep separates non-empty human message and BotData message.
	botDataSep = "\n\n"
)

// UnmarshalJSON sets `*at` to the `ActionType` the given bytes represents.
// Returns error for types other than `Start` or `Cancel`.
func (at *ActionType) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return errors.Fmt("unmarshal action: %w", err)
	}
	switch t := ActionType(s); t {
	case Start, Cancel:
		*at = t
		return nil
	default:
		return errors.Fmt("invalid action: %s", t)
	}
}

// ChangeID is the unique identifier for a Gerrit Change.
type ChangeID struct {
	// Host is the Gerrit host name for the Change.
	Host string
	// Number is the Gerrit Change number.
	Number int64
}

// MarshalText encodes `c` in the from of "c.Host:c.Number".
func (c *ChangeID) MarshalText() ([]byte, error) {
	return []byte(fmt.Sprintf("%s:%d", c.Host, c.Number)), nil
}

// UnmarshalText decodes the bytes and sets `*c` to the resurrected `ChangeID`.
func (c *ChangeID) UnmarshalText(b []byte) error {
	segs := strings.Split(string(b), ":")
	if len(segs) != 2 {
		return errors.Fmt("too many separators ':' in ChangeID text: %s", string(b))
	}
	num, err := strconv.ParseInt(segs[1], 10, 64)
	if err != nil {
		return errors.Fmt("invalid Change number: %s: %w", segs[1], err)
	}
	c.Host, c.Number = segs[0], num
	return nil
}

// BotData records an action taken by CV against a Gerrit Change.
type BotData struct {
	// Action describes the action taken by CV against a Gerrit Change.
	Action ActionType `json:"action"`
	// TriggeredAt is the timestamp when this action is triggered.
	TriggeredAt time.Time `json:"triggered_at"`
	// Revision is the revision (patch set) of the change at the time this
	// action is triggered.
	Revision string `json:"revision"`
}

// Parse tries to extract BotData from the given Gerrit Change message.
// Returns ok=false if no valid BotData is found in the message.
func Parse(cmi *gerritpb.ChangeMessageInfo) (ret BotData, ok bool) {
	if cmi == nil {
		return
	}
	idx := strings.Index(cmi.Message, botDataPrefix)
	if idx < 0 {
		return
	}
	if err := json.Unmarshal([]byte(cmi.Message[idx+len(botDataPrefix):]), &ret); err == nil {
		ok = true
	}
	return
}

// Append appends BotData message (in JSON form) to the given human message.
// The human message will be truncated if the length of result message exceeds
// `MaxMessageLength`. Returns error if marshalling BotData to JSON fails or
// the BotData message itself is already too long.
func Append(humanMsg string, bd BotData) (string, error) {
	return append(humanMsg, bd, gerrit.MaxMessageLength)
}

func append(humanMsg string, bd BotData, maxLen int) (string, error) {
	b, err := json.Marshal(bd)
	if err != nil {
		return "", errors.Fmt("marshal bot data: %w", err)
	}

	buf := bytes.NewBufferString(humanMsg)
	switch hmLen, sepLen, bdLen := len(humanMsg), len(botDataSep), len(botDataPrefix)+len(b); {
	case bdLen > maxLen:
		return "", errors.Fmt("bot data too long; max length: %d, got %d", maxLen, bdLen)
	case hmLen == 0:
		buf.Grow(bdLen)
		writeBotData(buf, b, false)
	case hmLen+sepLen+bdLen <= maxLen:
		buf.Grow(sepLen + bdLen)
		writeBotData(buf, b, true)
	default:
		keepLen := maxLen - bdLen - sepLen - len(gerrit.PlaceHolder)
		if keepLen <= 0 {
			return "", errors.Fmt("bot data too long to display human message; max length: %d, got %d", maxLen, bdLen)
		}
		buf.Truncate(keepLen)
		buf.Grow(maxLen - keepLen)
		buf.WriteString(gerrit.PlaceHolder)
		writeBotData(buf, b, true)
	}
	return buf.String(), nil
}

func writeBotData(buf *bytes.Buffer, bd []byte, addSep bool) {
	if addSep {
		buf.WriteString(botDataSep)
	}
	buf.WriteString(botDataPrefix)
	buf.Write(bd)
}
