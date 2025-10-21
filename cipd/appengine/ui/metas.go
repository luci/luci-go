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

package ui

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dustin/go-humanize"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/appengine/impl/model"
)

type instanceMetadataItem struct {
	Fingerprint     string
	Key             string
	User            string
	Age             string
	ContentType     string
	Size            string
	IsEmpty         bool   // there's no value
	IsText          bool   // the value can be displayed in a text area
	IsInlineText    bool   // the value is a simple short string
	TextValue       string // set only if IsText is true
	InlineTextValue string // slightly cleaned value for IsInlineText case
}

func instanceMetadataListing(md []*repopb.InstanceMetadata, now time.Time) []instanceMetadataItem {
	l := make([]instanceMetadataItem, len(md))
	for i, m := range md {
		l[i] = instanceMetadataItem{
			Fingerprint: m.Fingerprint,
			Key:         m.Key,
			User:        strings.TrimPrefix(m.AttachedBy, "user:"),
			Age:         humanize.RelTime(m.AttachedTs.AsTime(), now, "", ""),
			ContentType: m.ContentType,
			Size:        humanize.Bytes(uint64(len(m.Value))),
			IsEmpty:     len(m.Value) == 0,
		}
		if model.IsTextContentType(m.ContentType) {
			l[i].IsText = true
			l[i].TextValue = string(m.Value)
			if val, ok := prepAsInlineText(m.Value); ok {
				l[i].IsInlineText = true
				l[i].InlineTextValue = val
			}
		}
	}
	return l
}

func prepAsInlineText(v []byte) (s string, ok bool) {
	if len(v) > 400 {
		return "", false
	}

	s = string(v)
	if !utf8.ValidString(s) {
		return "", false
	}

	// Reject strings with new lines and other complicated spaces in the middle
	// of the string, allow only regular " ".
	s = strings.TrimSpace(s)
	for _, rune := range s {
		if unicode.IsSpace(rune) && rune != ' ' {
			return "", false
		}
	}

	return s, true
}
