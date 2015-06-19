// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolated

import (
	"encoding/json"
	"strconv"
)

// Int is a JSON/Cloud Endpoints friendly int type that will correctly parse
// string encoded integers found in JSON encoded data.
type Int int

func (i *Int) UnmarshalJSON(p []byte) error {
	val := 0
	if err := json.Unmarshal(p, &val); err == nil {
		*i = Int(val)
		return err
	}
	s := ""
	if err := json.Unmarshal(p, &s); err != nil {
		return err
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*i = Int(v)
	return nil
}

// ServerCapabilities is returned by /server_details.
type ServerCapabilities struct {
	ServerVersion string `json:"server_version"`
}

// DigestItem is used as input for /preupload via DigestCollection.
type DigestItem struct {
	Digest     HexDigest `json:"digest"`
	IsIsolated bool      `json:"is_isolated"`
	Size       int64     `json:"size"`
}

// DigestCollection is used as input for /preupload.
type DigestCollection struct {
	Items     []*DigestItem `json:"items"`
	Namespace struct {
		Namespace string `json:"namespace"`
	} `json:"namespace"`
}

// PreuploadStatus is returned by /preupload via URLCollection.
type PreuploadStatus struct {
	GSUploadURL  string `json:"gs_upload_url"`
	UploadTicket string `json:"upload_ticket"`
	Index        Int    `json:"index"`
}

// URLCollection is returned by /preupload.
type URLCollection struct {
	Items []PreuploadStatus `json:"items"`
}

// FinalizeRequest is used as input for /finalize_gs_upload.
type FinalizeRequest struct {
	UploadTicket string `json:"upload_ticket"`
}

// StorageRequest is used as input for /store_inline.
type StorageRequest struct {
	UploadTicket string `json:"upload_ticket"`
	Content      []byte `json:"content"`
}
