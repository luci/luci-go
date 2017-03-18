// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package utils

import (
	"crypto/sha256"
	"encoding/hex"
)

// TokenFingerprint returns first 16 bytes of SHA256 of the token, as hex.
//
// Token fingerprints can be used to identify tokens without parsing them.
func TokenFingerprint(tok string) string {
	digest := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(digest[:16])
}
