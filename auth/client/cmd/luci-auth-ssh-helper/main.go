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
	"go.chromium.org/luci/common/system/signals"
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

	switch f.Mode {
	case modeConnect:
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

	case modeDaemon:
		if err := standaloneMain(ctx, f); err != nil {
			logging.Errorf(ctx, "Daemon mode failed with: %v", err)
			os.Exit(2)
		}
	}
}

func createAgent(upstreamDialer ssh.AgentDialer, listener net.Listener) (*ssh.ExtensionAgent, error) {
	h := forwardingHandler{}
	if !h.available() {
		return nil, fmt.Errorf("No challenge handler available. Please set %v.", pluginEnvVar)
	}

	agent := ssh.ExtensionAgent{
		UpstreamDialer: upstreamDialer,
		Listener:       listener,
		Dispatcher: map[string]ssh.AgentExtensionHandler{
			reauth.SSHExtensionForwardedChallenge: h.handle,
		},
	}

	return &agent, nil
}

// Create an appropriate AgentDialer based on environment vars.
func createUpstreamDialer(ctx context.Context) ssh.AgentDialer {
	upstreamAddr := os.Getenv(reauth.EnvSSHAuthSock)
	if upstreamAddr == "" {
		logging.Warningf(ctx, "SSH_AUTH_SOCK isn't set.")
	} else {
		logging.Infof(ctx, "Proxies SSH agent commands to %v.", upstreamAddr)
	}

	return ssh.UpstreamWithFallback{Upstream: upstreamAddr}
}

func standaloneMain(ctx context.Context, r runArgs) error {
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%v", r.Port))
	if err != nil {
		return err
	}
	defer listener.Close()

	upstreamDialer := createUpstreamDialer(ctx)
	agent, err := createAgent(upstreamDialer, listener)
	if err != nil {
		return err
	}

	logging.Infof(ctx, "ReAuth handler listening on: %v", listener.Addr().String())

	signals.HandleInterrupt(func() { agent.Close() })

	// Blocks forever until agent.Close() is called.
	return agent.ListenLoop(ctx)
}

func sshHelperMain(ctx context.Context, r runArgs) error {
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

	upstreamDialer := createUpstreamDialer(ctx)
	agent, err := createAgent(upstreamDialer, listener)
	if err != nil {
		logging.Errorf(ctx, "Failed to create SSH agent: %v", err)
		return err
	}

	sshAuthSock := getSSHAuthSock(listener)
	logging.Infof(ctx, "LUCI SSH Agent listening on: %v", sshAuthSock)

	go agent.ListenLoop(ctx)
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
