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

package utils

import (
	"context"
	"fmt"

	"go.chromium.org/luci/server/auth/signing"
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
