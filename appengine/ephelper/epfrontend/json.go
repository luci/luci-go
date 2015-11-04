// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package epfrontend

import (
	"encoding/json"
)

type jsonRWRawMessage struct {
	v   interface{}
	raw []byte
}

var _ interface {
	json.Marshaler
	json.Unmarshaler
} = (*jsonRWRawMessage)(nil)

func (m *jsonRWRawMessage) MarshalJSON() ([]byte, error) {
	if m.raw != nil {
		return m.raw, nil
	}
	return json.Marshal(m.v)
}

func (m *jsonRWRawMessage) UnmarshalJSON(data []byte) error {
	m.raw = data
	return nil
}
