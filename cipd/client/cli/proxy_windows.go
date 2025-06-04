// Copyright 2025 The LUCI Authors.
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

//go:build windows
// +build windows

package cli

import (
	"context"
	"net/http"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd/proxyserver"
	"go.chromium.org/luci/cipd/client/cipd/proxyserver/proxypb"
)

func runProxyImpl(context.Context, string, *proxypb.Policy, *http.Client) (*proxyserver.ProxyStats, error) {
	return nil, errors.New("cipd proxy is not implemented on Windows")
}
