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

package gitsource

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"go.chromium.org/luci/lucicfg/internal/ui"
)

func isObjID(objName []byte) bool {
	n, err := hex.Decode(make([]byte, sha1.Size+1), objName)
	return err == nil && n == sha1.Size
}

// prefetchMultiple ensures that the objects (of any type) in objectNames are
// fetched locally.
//
// If they are already locally present, this is a fast noop.
func (r *repoCache) prefetchMultiple(ctx context.Context, objectNames []string, extraFlags ...string) error {
	if len(objectNames) == 0 {
		return nil
	}

	// Attempt 1:
	// You would hope that `cat-file --batch-check --buffer` would be sufficient
	// to efficiently fetch all these objects, but you would be incorrect (at
	// least as of git 2.48.1). Doing this will have `cat-file` serially issue
	// fetches for each individual object.
	//
	//   cmd := r.mkGitCmd(ctx, []string{
	//   	"cat-file", "--batch-check", "--buffer", "-Z"})
	//   buf := &bytes.Buffer{}
	//   for _, oid := range objectNames {
	//   	fmt.Fprintf(buf, "%s\x00", oid)
	//   }
	//   cmd.Stdin = buf
	//   return cmd.Run()

	// Attempt 2:
	// launch a separate `cat-file -e` for each object id from a pool :/...
	// But even this was quite slow.
	//
	//   grp, ctx := errgroup.WithContext(ctx)
	//   grp.SetLimit(2 * runtime.NumCPU())
	//   for _, oid := range objectNames {
	//   	grp.Go(func() error {
	//   		return r.git(ctx, "cat-file", "-e", oid)
	//   	})
	//   }
	//   return grp.Wait()

	// Attempt 3:
	//
	// Use cat-file to check which objects are missing, and then use `fetch` in
	// `--stdin` mode with negotiationAlgorithm set to noop. This will get fetch
	// to actually get a packfile with exactly the objects we need to fetch.
	//
	// This still has quite a bit of overhead for large repos like chromium/src,
	// however, taking about ~20s for a couple hundred objects.
	cmd := r.mkGitCmd(ctx, []string{"--no-lazy-fetch", "cat-file", `--batch-check=OK`, "-Z"})
	cmd.Stdout = nil // may be set by mkGitCmd
	cmd.Stdin = strings.NewReader(strings.Join(objectNames, "\x00") + "\x00")
	result, err := cmd.Output()
	if err != nil {
		return err
	}
	reader := bufio.NewReader(bytes.NewReader(result))

	// toFetch will be the buffered list of object IDs separated by newlines, with
	// a trailing newline, to pass to fetch on stdin.
	var toFetch []byte
	var fetchCount int
	for {
		line, err := reader.ReadBytes(0)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		// we expect the line to look like either:
		//
		//   OK <NUL>
		//   original object spec <SP> missing <NUL>
		if bytes.Equal(line, []byte("OK\x00")) {
			continue
		}
		if trimmed := bytes.TrimSuffix(line, []byte(" missing\x00")); len(trimmed) < len(line) {
			resolved := trimmed
			if !isObjID(resolved) { // skip calling rev-parse if the request was already an object ID
				// Unfortunately, cat-file regurgitates our original object names in this
				// case, which can be things like:
				//
				//   <commit>:path/to/blob
				//
				// For our fetch call below, we will need to have these resolved to
				// a concrete objectid, because `git fetch` will only negotiate in terms
				// of raw object hashes.
				resolved, err = r.gitOutput(ctx, "rev-parse", string(resolved))
				if err != nil {
					return fmt.Errorf("rev-parse %q: %w", trimmed, err)
				}
				resolved = bytes.TrimSuffix(resolved, []byte("\n"))
			}

			toFetch = append(toFetch, resolved...)
			toFetch = append(toFetch, '\n')
			fetchCount++
		} else {
			return fmt.Errorf("unexpected output from cat-file: %q", line)
		}
	}

	if len(toFetch) > 0 {
		// now fire off a single fetch to pull all of these hashes in.
		if fetchCount == 1 {
			ui.ActivityProgress(ctx, "fetching 1 object")
		} else {
			ui.ActivityProgress(ctx, "fetching %d objects", fetchCount)
		}
		args := []string{
			"-c", "fetch.negotiationAlgorithm=noop",
			"fetch", "origin", "--no-tags", "--no-write-fetch-head",
			"--stdin"}
		args = append(args, extraFlags...)
		cmd = r.mkGitCmd(ctx, args)
		cmd.Stdin = bytes.NewReader(toFetch)
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	return nil
}
