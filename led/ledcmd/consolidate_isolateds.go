// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package ledcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"

	"go.chromium.org/luci/client/archiver"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/led/job"
	api "go.chromium.org/luci/swarming/proto/api"
)

// ConsolidateIsolateSources will, for Swarming tasks:
//
//   * Extract Cmd/Cwd from the Properties.CasInputs (if set)
//   * Combine the Properties.CasInputs with the UserPayload (if set)
//   * Store the combined isolated in Properties.CasInputs.
func ConsolidateIsolateSources(ctx context.Context, authClient *http.Client, jd *job.Definition) error {
	if jd.GetSwarming() == nil {
		return nil
	}

	arc := mkArchiver(ctx, mkIsoClient(authClient, jd.UserPayload))

	for _, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
		if slc.Properties == nil {
			slc.Properties = &api.TaskProperties{}
		}
		if slc.Properties.CasInputs == nil {
			slc.Properties.CasInputs = jd.UserPayload
			return nil
		}

		// extract the cmd/cwd from the isolated, if they're set.
		//
		// This is an old feature of swarming/isolated where the isolated file can
		// contain directives for the swarming task.
		cmd, cwd, err := extractCmdCwdFromIsolated(ctx, authClient, slc.Properties.CasInputs)
		if err != nil {
			return err
		}
		if len(cmd) > 0 {
			slc.Properties.Command = cmd
			slc.Properties.RelativeCwd = cwd
			// ExtraArgs is allowed to be set only if the Command is coming from the
			// isolated. However, now that we're explicitly setting the Command, we
			// must move ExtraArgs into Command.
			if len(slc.Properties.ExtraArgs) > 0 {
				slc.Properties.Command = append(slc.Properties.Command, slc.Properties.ExtraArgs...)
				slc.Properties.ExtraArgs = nil
			}
		}

		if jd.UserPayload == nil {
			continue
		}

		slc.Properties.CasInputs, err = combineIsolateds(ctx, arc,
			jd.UserPayload, slc.Properties.CasInputs)
		return errors.Annotate(err, "combining isolateds").Err()
	}
	return nil
}

func extractCmdCwdFromIsolated(ctx context.Context, authClient *http.Client, rootIso *api.CASTree) (cmd []string, cwd string, err error) {
	seenIsolateds := map[isolated.HexDigest]struct{}{}
	queue := isolated.HexDigests{isolated.HexDigest(rootIso.Digest)}
	isoClient := mkIsoClient(authClient, rootIso)

	// borrowed from go.chromium.org/luci/client/downloader.
	//
	// It's rather silly that there's no library functionality to do this.
	for len(queue) > 0 {
		iso := queue[0]
		if _, ok := seenIsolateds[iso]; ok {
			err = errors.Reason("loop detected when resolving isolate %q", rootIso).Err()
			return
		}
		seenIsolateds[iso] = struct{}{}

		buf := bytes.Buffer{}
		if err = isoClient.Fetch(ctx, iso, &buf); err != nil {
			err = errors.Annotate(err, "fetching isolated %q", iso).Err()
			return
		}
		isoFile := isolated.Isolated{}
		if err = json.Unmarshal(buf.Bytes(), &isoFile); err != nil {
			err = errors.Annotate(err, "parsing isolated %q", iso).Err()
			return
		}

		if len(isoFile.Command) > 0 {
			cmd = isoFile.Command
			cwd = isoFile.RelativeCwd
			break
		}

		queue = append(isoFile.Includes, queue[1:]...)
	}

	return
}

func mkArchiver(ctx context.Context, isoClient *isolatedclient.Client) *archiver.Archiver {
	// The archiver is pretty noisy at Info level, so we skip giving it
	// a logging-enabled context unless the user actually requseted verbose.
	arcCtx := context.Background()
	if logging.GetLevel(ctx) < logging.Info {
		arcCtx = ctx
	}
	// os.Stderr will cause the archiver to print a one-liner progress status.
	return archiver.New(arcCtx, isoClient, os.Stderr)
}

func combineIsolateds(ctx context.Context, arc *archiver.Archiver, isos ...*api.CASTree) (*api.CASTree, error) {
	server, ns := "", ""
	digests := isolated.HexDigests{}
	for _, iso := range isos {
		if iso == nil {
			continue
		}

		if server == "" {
			server = iso.Server
		} else if iso.Server != server {
			return nil, errors.Reason(
				"cannot combine isolates from two different servers: %q vs %q",
				server, iso.Server).Err()
		}

		if ns == "" {
			ns = iso.Namespace
		} else if iso.Namespace != ns {
			return nil, errors.Reason(
				"cannot combine isolates from two different namespaces: %q vs %q",
				ns, iso.Namespace).Err()
		}

		digests = append(digests, isolated.HexDigest(iso.Digest))
	}
	ret := &api.CASTree{
		Server:    server,
		Namespace: ns,
	}
	if len(digests) == 1 {
		ret.Digest = string(digests[0])
		return ret, nil
	} else if len(digests) == 0 {
		return nil, nil
	}

	iso := isolated.New(isolated.GetHash(isos[0].Namespace))
	iso.Includes = digests
	isolated, err := json.Marshal(iso)
	if err != nil {
		return nil, errors.Annotate(err, "encoding ISOLATED.json").Err()
	}
	promise := arc.Push("ISOLATED.json", isolatedclient.NewBytesSource(isolated), 0)
	promise.WaitForHashed()
	ret.Digest = string(promise.Digest())

	return ret, arc.Close()
}

func mkIsoClient(authClient *http.Client, tree *api.CASTree) *isolatedclient.Client {
	return isolatedclient.New(
		nil, authClient,
		tree.Server, tree.Namespace,
		retry.Default,
		nil,
	)
}
