// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package utils

import (
	"fmt"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/signing"
)

// ServiceVersion returns a string that identifies the app and the version.
//
// It is put in some server responses. The function extracts this information
// from the given signer.
//
// This function almost never returns errors. It can return an error only when
// called for the first time during the process lifetime. It gets cached after
// first successful return.
func ServiceVersion(c context.Context, s signing.Signer) (string, error) {
	inf, err := s.ServiceInfo(c) // cached
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", inf.AppID, inf.AppVersion), nil
}
