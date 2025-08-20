// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"os"
	"path"

	luciflag "go.chromium.org/luci/common/flag"
)

// SSH Helper mode.
type helperMode string

const (
	// Acts as an SSH binary wrapper.
	//
	// Starts an SSH agent that listens for ReAuth challenges, then starts an
	// `ssh` session using this SSH agent.
	//
	// Other SSH agent commands are proxied to SSH_AUTH_SOCK, if it's set.
	modeConnect helperMode = "connect"

	// Runs as a daemon (i.e. doesn't start an SSH session) and starts an SSH
	// agent that listens for ReAuth challenges.
	//
	// Other SSH agent commands are proxied to SSH_AUTH_SOCK, if it's set.
	modeDaemon helperMode = "daemon"
)

type runArgs struct {
	// This helper's operation mode.
	Mode helperMode

	// If set, logs debug info to os.Stderr. This will mess up an interactive
	// SSH tty.
	Verbose bool

	// For `modeStandalone`, the TCP port to listen on.
	//
	// For security reason, the TCP listener always binds to `localhost`.
	Port int

	// Args to be passed to the `ssh`` binary.
	SSHArgs []string
}

func (f *runArgs) registerFlags(fs *flag.FlagSet) {
	fs.Var(
		luciflag.NewChoice(
			(*string)(&f.Mode),
			string(modeConnect),
			string(modeDaemon),
		),
		"mode",
		fmt.Sprintf("Operation mode, choose from: %v (default), %v", modeConnect, modeDaemon),
	)

	fs.IntVar(
		&f.Port,
		"port",
		0,
		fmt.Sprintf("When running with -mode=%v, the TCP port to listen on for incoming connections. The port always binds to `localhost`.", modeDaemon),
	)

	fs.BoolVar(&f.Verbose, "verbose", false, "Enables verbose logging. This might mess up the SSH terminal output")
}

// parseArgs parses command line arguments (usually os.Args).
//
// This method prints error and usage info to os.Stderr as appropriate.
//
// Returns flag.ErrHelp if help usage is requested
// Returns an error if the arguments are invalid.
func parseArgs(osArgs []string) (runArgs, error) {
	var r runArgs

	arg0, args := path.Base(osArgs[0]), osArgs[1:]
	fs := flag.NewFlagSet(arg0, flag.ContinueOnError)
	r.registerFlags(fs)

	// Add usage examples.
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "%v receives ReAuth security key challenges from an SSH session.\n\n", arg0)
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  * To replace `ssh` command: `%v -- [ssh_opts] [user@]host`\n", arg0)
		fmt.Fprintf(os.Stderr, "    You must use \"--\" to mark the beginning of ssh arguments.\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "  * To use this tool on its own: %v -mode=daemon -port=<PORT>\n", arg0)
		fmt.Fprintf(os.Stderr, "    This opens a TCP listener on localhost:<PORT> to handle ReAuth challenge.\n")
		fmt.Fprintf(os.Stderr, "    You need to setup port forwarding and send ReAuth challenges to the above port.\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "If SSH_AUTH_SOCK is set, this program proxies SSH agent commands it receives to SSH_AUTH_SOCK.\n")
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return r, err
	}

	// Defaults to passthrough mode.
	if r.Mode == "" {
		r.Mode = modeConnect
	}

	// Check port range.
	if r.Port < 0 || r.Port > 65535 {
		return r, fmt.Errorf("Invalid port: %v, port must be within range [0, 65535].", r.Port)
	}

	r.SSHArgs = fs.Args()

	return r, nil
}
