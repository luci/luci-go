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

package rawpresentation

import (
	"net/http"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/grpc/prpc"
	"github.com/luci/luci-go/logdog/client/coordinator"
	"github.com/luci/luci-go/server/auth"

	"golang.org/x/net/context"
)

func resolveHost(host string) (string, error) {
	// Resolveour our Host, and validate it against a host whitelist.
	switch host {
	case "":
		return defaultLogDogHost, nil
	case defaultLogDogHost, "luci-logdog-dev.appspot.com":
		return host, nil
	default:
		return "", errors.Reason("host %q is not whitelisted", host).Err()
	}
}

// NewClient generates a new LogDog client that issues requests on behalf of the
// current user.
func NewClient(c context.Context, host string) (*coordinator.Client, error) {
	var err error
	if host, err = resolveHost(host); err != nil {
		return nil, err
	}

	// Initialize the LogDog client authentication.
	t, err := auth.GetRPCTransport(c, auth.AsUser)
	if err != nil {
		return nil, errors.New("failed to get transport for LogDog server")
	}

	// Setup our LogDog client.
	return coordinator.NewClient(&prpc.Client{
		C: &http.Client{
			Transport: t,
		},
		Host: host,
	}), nil
}
