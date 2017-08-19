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

package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/fetcher"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/luci_config/common/cfgtypes"
)

var errNoAuth = errors.New("no access")

type streamInfo struct {
	Project cfgtypes.ProjectName
	Path    types.StreamPath

	// Client is the HTTP client to use for LogDog communication.
	Client *coordinator.Client
}

const noStreamDelay = 5 * time.Second

func logHandler(c context.Context, w http.ResponseWriter, host, path string) error {
	// TODO(hinoka): Move this to luci-config.
	if !(host == "luci-logdog.appspot.com" || host == "luci-logdog-dev.appspot.com") {
		return fmt.Errorf("unknown host %s", host)
	}
	spath := strings.SplitN(path, "/", 2)
	if len(spath) != 2 {
		return fmt.Errorf("%s is not a valid path", path)
	}
	project := cfgtypes.ProjectName(spath[0])
	streamPath := types.StreamPath(spath[1])

	client := coordinator.NewClient(&prpc.Client{
		C: &http.Client{
			// TODO(hinoka): Once crbug.com/712506 is resolved, figure out how to get auth.
			Transport: http.DefaultTransport,
		},
		Host: host,
	})
	stream := client.Stream(project, streamPath)

	// Pull stream information.
	f := stream.Fetcher(c, &fetcher.Options{
		// Try to buffer as much as possible, with a large window, since this is
		// basically a cloud-to-cloud connection.
		BufferCount:    200,
		BufferBytes:    int64(4 * 1024 * 1024),
		PrefetchFactor: 10,
	})

	for {
		// Read out of the buffer.  This _should_ be bottlenecked on the network
		// connection between the Flex instance and the client, via Fprintf().
		entry, err := f.NextLogEntry()
		switch err {
		case io.EOF:
			return nil // We're done.
		case nil:
			// Nothing
		case coordinator.ErrNoAccess:
			return errNoAuth // This will force a redirect
		default:
			return err
		}
		content := entry.GetText()
		if content == nil {
			break
		}
		for _, line := range content.Lines {
			fmt.Fprint(w, line.Value)
			fmt.Fprint(w, line.Delimiter)
		}
	}
	return nil
}
