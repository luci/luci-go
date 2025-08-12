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
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"go.chromium.org/luci/auth/reauth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/ssh"
)

func main() {
	f, err := parseArgs(os.Args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		} else {
			os.Exit(2)
		}
	}

	ctx := context.Background()
	ctx = gologger.StdConfig.Use(ctx)
	if f.Verbose {
		ctx = logging.SetLevel(ctx, logging.Debug)
	} else {
		ctx = logging.SetLevel(ctx, logging.Warning)
	}

	if err := sshHelperMain(ctx, f); err != nil {
		// Propagate exit code if available.
		if err, ok := err.(*exec.ExitError); ok {
			os.Exit(err.ExitCode())
		}

		// Other errors.
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func sshHelperMain(ctx context.Context, r runArgs) error {
	upstreamAddr, _ := os.LookupEnv(reauth.EnvSSHAuthSock)
	if upstreamAddr == "" {
		logging.Warningf(ctx, "SSH_AUTH_SOCK isn't set.")
	} else {
		logging.Infof(ctx, "Local SSH_AUTH_SOCK is %v", upstreamAddr)
	}

	upstreamDialer := ssh.UpstreamWithFallback{Upstream: upstreamAddr}

	listenerDir, err := os.MkdirTemp("", "luci_ssh_agent.")
	if err != nil {
		logging.Errorf(ctx, "Failed to create temporary directory: %v", err)
		return err
	}
	defer os.RemoveAll(listenerDir)

	socketPath := filepath.Join(listenerDir, "ssh_auth_sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return errors.WrapIf(err, "failed to create agent listener")
	}
	defer listener.Close()

	sshAuthSock := getSSHAuthSock(listener)
	logging.Infof(ctx, "LUCI SSH Agent listening on: %v", sshAuthSock)

	agent := ssh.ExtensionAgent{
		UpstreamDialer: upstreamDialer,
		Listener:       listener,
		Dispatcher: map[string]ssh.AgentExtensionHandler{
			reauth.SSHExtensionForwardedChallenge: forwardedChallengeHandler,
		},
	}

	listenerCtx := ctx

	go agent.ListenLoop(listenerCtx)
	defer agent.Close()

	// Start the real SSH command.
	sshPath, err := exec.LookPath("ssh")
	if err != nil {
		log.Printf("SSH binary not found: %v", err)
	}
	sshArgs := []string{
		// Inject agent forwarding flag.
		"-o", "ForwardAgent=yes",

		// Don't use existing Control channels. If set to yes, SSH client will
		// reuse an existing SSH session that isn't using the helper's
		// SSH_AUTH_SOCK.
		"-o", "ControlMaster=no",
		"-o", "ControlPath=no",
	}
	sshArgs = append(sshArgs, r.SSHArgs...)

	logging.Infof(ctx, "Running SSH with args: %#v", sshArgs)

	cmd := exec.Command(sshPath, sshArgs...)
	cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("SSH_AUTH_SOCK=%s", sshAuthSock),
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Start()

	return cmd.Wait()
}

// getSSHAuthSock returns a string that should be passed as the SSH_AUTH_SOCK
// environment variable to ssh executable.
func getSSHAuthSock(l net.Listener) string {
	a := l.Addr()
	switch a.Network() {
	case "unix":
		return a.String()
	default:
		// We shouldn't be creating a listener that can't run agent server.
		// For example, udp or unixgram.
		panic(fmt.Sprintf("Listener type %v can't be used.", a.Network()))
	}
}
