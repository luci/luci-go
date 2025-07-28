// Copyright 2024 The LUCI Authors.
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

// Package replicas contains functionality to communicate with legacy
// services that rely on the "direct push" method of AuthDB replication.
package replicas

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth"
)

// SendAuthDB sends the AuthDB blob to the given replica, returning its
// response.
func SendAuthDB(ctx context.Context, replicaURL, keyName, encodedSig string, authDBBlob []byte) (*http.Response, error) {
	replicaURL = strings.TrimRight(replicaURL, "/")
	var schema string
	if info.IsDevAppServer(ctx) {
		schema = "http://"
	} else {
		schema = "https://"
	}
	if !strings.HasPrefix(replicaURL, schema) {
		return nil, fmt.Errorf("replica URL is '%s'; must explicitly use schema '%s'",
			replicaURL, schema)
	}

	// Create the request to the replica, targeting the AuthDB replication
	// endpoint.
	req, err := http.NewRequest(
		"POST",
		replicaURL+"/auth/api/v1/internal/replication",
		bytes.NewReader(authDBBlob))
	if err != nil {
		return nil, errors.Fmt("failed creating http.Request to %s: %w", replicaURL, err)
	}

	// Pass signature via the header.
	req.Header = http.Header{
		"Content-Type":       []string{"application/octet-stream"},
		"X-AuthDB-SigKey-v1": []string{keyName},
		"X-AuthDB-SigVal-v1": []string{encodedSig},
	}

	// Set a 60 sec timeout.
	c, cancel := clock.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Send the request as Auth Service.
	tr, err := auth.GetRPCTransport(c, auth.AsSelf)
	if err != nil {
		return nil, errors.Fmt("error getting transport: %w", err)
	}
	client := &http.Client{
		Transport: tr,
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return res, nil
}
