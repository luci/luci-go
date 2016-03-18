// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package admin

import (
	"encoding/pem"
	"fmt"
)

// parsePEM takes pem-encoded block and decodes it, checking the header.
func parsePEM(data, header string) ([]byte, error) {
	block, rest := pem.Decode([]byte(data))
	if len(rest) != 0 || block == nil {
		return nil, fmt.Errorf("not a valid %q PEM", header)
	}
	if block.Type != header {
		return nil, fmt.Errorf("expecting %q, got %q", header, block.Type)
	}
	return block.Bytes, nil
}

// dumpPEM transforms block to pem-encoding.
//
// Reverse of parsePEM.
func dumpPEM(data []byte, header string) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  header,
		Bytes: data,
	}))
}
