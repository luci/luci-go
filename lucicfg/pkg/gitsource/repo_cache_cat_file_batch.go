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
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/lucicfg/pkg/source"
)

type batchProc struct {
	// mkCmd is kind of a weird circular dependency with RepoCache - this is the
	// case because currently batchProc can (theoretically) recover from a crash
	// of `git cat-file --batch` and recreate the command.
	mkCmd func(ctx context.Context, args []string) *exec.Cmd

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

// shutdown will terminate the contained process, if there is one.
//
// Calling any other method on this will spawn a new process.
func (b *batchProc) shutdown() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.shutdownLocked()
}

func (b *batchProc) shutdownLocked() {
	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
	}
	if b.stdin != nil {
		b.stdin.Close()
	}
	b.cmd = nil
	b.stdin = nil
	b.stdout = nil
}

func parseCatFileResponse(stdout *bufio.Reader) (kind source.ObjectKind, data []byte, err error) {
	// first, pull through to the next NULL - this is our status line
	statusBytes, err := stdout.ReadBytes(0)
	if err != nil {
		return
	}

	status := string(statusBytes[:len(statusBytes)-1]) // trim off the NULL at the end

	// next, check the end of the status line. This will tell us if we are in an
	// error condition of some kind.
	if strings.HasSuffix(status, " missing") {
		err = source.ErrMissingObject
		return
	}
	if strings.HasSuffix(status, " ambiguous") {
		// these have no additional data on the line, just return these directly as
		// an error.
		err = fmt.Errorf("%q", status)
		return
	}

	// All other types end with ... <size> NULL, so read that out now.
	tokens := strings.Split(status, " ")
	if len(tokens) < 2 {
		err = fmt.Errorf("could not parse status, expected >= 2 tokens: %v", tokens)
		return
	}

	size, err := strconv.ParseInt(tokens[len(tokens)-1], 10, 64)
	if err != nil {
		err = fmt.Errorf("could not parse size %q: %w", tokens[len(tokens)-1], err)
		return
	}

	data = make([]byte, size)
	_, err = io.ReadFull(stdout, data)
	if err != nil {
		err = fmt.Errorf("could not read full data: %w", err)
		return
	}

	byt, err := stdout.ReadByte()
	if err != nil {
		err = fmt.Errorf("could not read terminal NULL: %w", err)
		return
	}
	if byt != 0 {
		err = fmt.Errorf("non-NULL followed object data?")
		return
	}
	kind = source.ParseKind(tokens[len(tokens)-2])
	return
}

func (b *batchProc) ensureCatFileBatchProcLocked(ctx context.Context) error {
	if b.cmd == nil {
		b.cmd = b.mkCmd(context.WithoutCancel(ctx), []string{"--no-lazy-fetch", "cat-file", "--batch", "-Z", "--follow-symlinks"})
		b.cmd.Stdout = nil // may be set by mkCmd

		var err error
		b.stdin, err = b.cmd.StdinPipe()
		if err != nil {
			return err
		}

		var stdoutPipe io.Reader
		stdoutPipe, err = b.cmd.StdoutPipe()
		if err != nil {
			return err
		}
		b.stdout = bufio.NewReader(stdoutPipe)

		if err = b.cmd.Start(); err != nil {
			return err
		}
	}
	return nil
}

func (b *batchProc) catFile(ctx context.Context, request string) (kind source.ObjectKind, data []byte, err error) {
	kind, data, err = func() (kind source.ObjectKind, data []byte, err error) {
		b.mu.Lock()
		defer b.mu.Unlock()

		if err = b.ensureCatFileBatchProcLocked(ctx); err != nil {
			return
		}

		stdin := b.stdin
		stdout := b.stdout

		// readDone indicates to that the 'shutdowner' goroutine below that the read
		// completed and it doesn't need to take any action.
		readDone := make(chan struct{})

		// shutdownerDone indicates that the 'shutdowner' goroutine below has
		// completed.
		shutdownerDone := make(chan struct{})

		// ensure that we don't release b.mu until the shutdowner goroutine has
		// terminated.
		defer func() {
			<-shutdownerDone
		}()

		proc := b.cmd.Process

		// start the shutdowner goroutine
		go func() {
			defer close(shutdownerDone)
			select {
			case <-ctx.Done():
				proc.Kill()
			case <-readDone:
			}
		}()

		// terminate the shutdowner goroutine, shutting down the catFile subprocess
		// if we're returning an error, which could indicate a partial read.
		defer func() {
			defer close(readDone)
			// to limit raciness, if context is done, always use that cause.
			if cause := context.Cause(ctx); cause != nil {
				err = cause
			}
			if err != nil {
				b.shutdownLocked()
			}
		}()

		// actually perform the write+read
		//
		// If we panic, the program is doomed, but we will release the lock.
		//
		// If we read successfully (no error) then we will:
		//   * skip calling shutdownLocked
		//   * close readDone, causing shutdowner goroutine to exit
		//   * wait for shutdownerDone
		//   * release b.mu
		//
		// If we read unsuccessfully (err != nil) then we will:
		//   * call shutdownLocked (for non nil error)
		//   * close readDone, causing shutdowner goroutine to exit
		//   * wait for shutdownerDone
		//   * release b.mu
		//
		// If context is canceled
		//   * shutdowner goroutine will kill the subprocess (
		//     but not touch `b` to avoid racing) and close shutdownerDone.
		//   * non-err MAY be returned from fmt.Fprintf or parseCatFileResponse.
		//   * We upgrade nil error to ctx.Err()
		//   * call shutdownLocked (for non nil error)
		//   * close readDone (no effect)
		//   * wait for shutdownerDone (already done)
		//   * release b.mu
		logging.Debugf(ctx, "catFile %s", request)
		_, err = fmt.Fprintf(stdin, "%s\x00", request)
		if err != nil {
			if ctx.Err() != nil {
				err = context.Cause(ctx)
			}
			return
		}
		kind, data, err = parseCatFileResponse(stdout)
		if err != nil {
			if ctx.Err() != nil {
				err = context.Cause(ctx)
			} else {
				err = fmt.Errorf("bad cat-file request: %q: %w", request, err)
			}
		}
		return
	}()
	if err != nil {
		return source.UnknownKind, nil, err
	}

	// Handle error cases:
	switch kind {
	case source.SymlinkKind:
		err = fmt.Errorf("symlink points out of repo: %q", string(data))
	case source.DanglingSymlinkKind:
		err = fmt.Errorf("symlink (transitively?) points to a missing file: %q", string(data))
	case source.LoopingSymlinkKind:
		err = fmt.Errorf("symlink (transitively?) points to itself (or has >40 hops): %q", string(data))
	case source.NotDirSymlinkKind:
		err = fmt.Errorf("symlink (transitively?) uses file as directory: %q", string(data))
	}
	return
}
