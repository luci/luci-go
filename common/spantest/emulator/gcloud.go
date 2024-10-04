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

package emulator

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
)

// addrRe is what we sniff out of the logs to know the server port.
var addrRe = regexp.MustCompile(`.*Server address\: 127\.0\.0\.1\:(\d+)$`)

// startViaGcloud attempts to launch Cloud Spanner emulator native binary.
func startViaGcloud(ctx context.Context, out io.Writer) (grpcAddr string, stop func(), err error) {
	bin, err := findEmulatorPath()
	if err != nil {
		return "", nil, err
	}

	ctx, cancel := context.WithCancel(ctx)

	// Sniffs the logs to recognize "Server address: 127.0.0.1:xxxxx" line.
	// This is unfortunately the only simple way to figure out what port the
	// emulator is serving on.
	sniffer := &logSniffer{
		out:     out,
		sniffed: make(chan string, 1),
	}

	// Ask the emulator to bind to an OS-assigned port. It will log it.
	cmd := exec.CommandContext(ctx, bin, "--host_port", "127.0.0.1:0")
	cmd.Stdout = sniffer
	cmd.Stderr = sniffer
	setPdeathsig(cmd) // ask Linux to kill it if the current process dies
	if err = cmd.Start(); err != nil {
		cancel()
		return "", nil, err
	}

	stop = func() {
		cancel()
		_ = cmd.Wait()
	}

	// Kill if exiting with an error or a panic.
	done := false
	defer func() {
		if err != nil || !done {
			stop()
		}
	}()

	// Wait until the server port is reported in the log.
	select {
	case port := <-sniffer.sniffed:
		if port == "" {
			err = errors.Reason("unexpected error writing into the log pipe").Err()
			return
		}
		grpcAddr = fmt.Sprintf("127.0.0.1:%s", port)
	case <-clock.After(ctx, 30*time.Second):
		err = errors.Reason("timeout waiting for the emulator to log its serving port").Err()
		return
	}

	done = true
	return
}

type logSniffer struct {
	out      io.Writer
	sniffed  chan string
	finished bool
	line     []byte
}

func (s *logSniffer) Write(p []byte) (n int, err error) {
	n, err = s.out.Write(p)
	if err != nil {
		s.done("")
		return
	}
	for len(p) != 0 {
		if s.finished {
			return
		}
		if eol := bytes.IndexByte(p, '\n'); eol != -1 {
			s.line = append(s.line, p[:eol]...)
			s.flushLine()
			p = p[eol+1:]
		} else {
			s.line = append(s.line, p...)
			p = nil
		}
	}
	return
}

func (s *logSniffer) flushLine() {
	if match := addrRe.FindSubmatch(s.line); len(match) != 0 {
		s.done(string(match[1]))
	} else {
		s.line = s.line[:0]
	}
}

func (s *logSniffer) done(port string) {
	if !s.finished {
		s.finished = true
		s.line = nil
		s.sniffed <- port
	}
}

var emulatorPath struct {
	once sync.Once
	path string
	err  error
}

// findEmulatorPath finds the path to Cloud Spanner emulator binary.
func findEmulatorPath() (string, error) {
	emulatorPath.once.Do(func() {
		emulatorPath.path, emulatorPath.err = func() (string, error) {
			o, err := exec.Command("gcloud", "info", "--format=value(installation.sdk_root)").Output()
			if err != nil {
				return "", err
			}
			sdkRoot := strings.TrimSpace(string(o))
			path := filepath.Join(sdkRoot, "bin/cloud_spanner_emulator/emulator_main")
			switch _, err = os.Stat(path); {
			case os.IsNotExist(err):
				return "", errors.Reason("gcloud SDK at %q doesn't have cloud-spanner-emulator component installed", sdkRoot).Err()
			case err != nil:
				return "", err
			}
			return path, nil
		}()
	})
	return emulatorPath.path, emulatorPath.err
}
