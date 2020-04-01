// Copyright 2020 The LUCI Authors.
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

package ledcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"

	"go.chromium.org/luci/client/archiver"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	api "go.chromium.org/luci/swarming/proto/api"
)

var isolateTestIfaceKey = "holds an isoClientIface during tests"

type pendingItem interface {
	WaitForHashed()
	Digest() isolated.HexDigest
}

type isoClientIface interface {
	io.Closer

	Fetch(context.Context, isolated.HexDigest, io.Writer) error
	Push(filename string, data isolatedclient.Source, priority int64) pendingItem

	Server() string
	Namespace() string
}

type prodIsoClient struct {
	raw *isolatedclient.Client
	arc *archiver.Archiver

	server    string
	namespace string
}

func (p *prodIsoClient) Close() error      { return p.arc.Close() }
func (p *prodIsoClient) Server() string    { return p.server }
func (p *prodIsoClient) Namespace() string { return p.namespace }
func (p *prodIsoClient) Push(filename string, data isolatedclient.Source, priority int64) pendingItem {
	return p.arc.Push(filename, data, priority)
}
func (p *prodIsoClient) Fetch(ctx context.Context, iso isolated.HexDigest, w io.Writer) error {
	return p.raw.Fetch(ctx, iso, w)
}

func mkIsoClient(ctx context.Context, authClient *http.Client, tree *api.CASTree) isoClientIface {
	if val, ok := ctx.Value(&isolateTestIfaceKey).(isoClientIface); ok {
		return val
	}

	client := isolatedclient.New(
		nil, authClient,
		tree.Server, tree.Namespace,
		retry.Default,
		nil,
	)
	// The archiver is pretty noisy at Info level, so we skip giving it
	// a logging-enabled context unless the user actually requseted verbose.
	arcCtx := context.Background()
	if logging.GetLevel(ctx) < logging.Info {
		arcCtx = ctx
	}
	// os.Stderr will cause the archiver to print a one-liner progress status.
	return &prodIsoClient{
		client,
		archiver.New(arcCtx, client, os.Stderr),
		tree.Server,
		tree.Namespace,
	}
}

func fetchIsolated(ctx context.Context, arc isoClientIface, dgst isolated.HexDigest) (*isolated.Isolated, error) {
	buf := bytes.Buffer{}
	if err := arc.Fetch(ctx, dgst, &buf); err != nil {
		return nil, errors.Annotate(err, "fetching isolated %q", dgst).Err()
	}
	isoFile := isolated.Isolated{}
	if err := json.Unmarshal(buf.Bytes(), &isoFile); err != nil {
		return nil, errors.Annotate(err, "parsing isolated %q", dgst).Err()
	}
	return &isoFile, nil
}
