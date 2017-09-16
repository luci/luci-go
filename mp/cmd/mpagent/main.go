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

// Package main contains the Machine Provider Agent, a process which runs on
// machines and communicates with the Machine Provider service.
package main

import (
	"flag"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"cloud.google.com/go/compute/metadata"

	"github.com/kardianos/osext"

	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/logging/teelogger"

	"golang.org/x/net/context"
)

// Platform-specific implementation. See agent_*.go.
type Strategy interface {
	// chown changes ownership of a path.
	chown(ctx context.Context, username string, path string) error
	// start starts the agent.
	start(ctx context.Context, path string) error
	// stop stops all instances of the agent.
	stop(ctx context.Context) error
	// reboot reboots the machine.
	reboot(ctx context.Context) error
}

type Agent struct {
	// Path to install this agent's auto-start config to.
	agentAutoStartPath string
	// Name of the file containing this agent's auto-start template.
	agentAutoStartTemplate string
	// Directory where this agent should emit logging output.
	logsDir string
	// Path to install this agent's Swarming auto-start config to.
	swarmingAutoStartPath string
	// Name of the file containing this agent's Swarming auto-start template.
	swarmingAutoStartTemplate string
	// Directory where this agent should install the Swarming bot process.
	swarmingBotDir string
	// Name of the user the agent should install Swarming for.
	swarmingUser string
	// Platform-specific implementation for this agent.
	strategy Strategy
}

// Configures logging.
//
// Returns modified context.Context.
func (agent *Agent) configureLogging(ctx context.Context) (context.Context, error) {
	if err := os.MkdirAll(agent.logsDir, 0755); err != nil {
		return ctx, err
	}

	// TODO(smut): Capture logging emitted before configureLogging was called and
	// write it to the log file.
	out, err := os.OpenFile(filepath.Join(agent.logsDir, "agent.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return ctx, err
	}
	config := gologger.LoggerConfig{Out: out}
	return teelogger.Use(ctx, config.NewLogger), nil
}

// Configures auto-connect to the given Swarming server on reboot.
func (agent *Agent) configureSwarmingAutoStart(ctx context.Context, serviceAccount string, server string) error {
	if err := os.MkdirAll(agent.swarmingBotDir, 0755); err != nil {
		return err
	}
	if err := agent.strategy.chown(ctx, agent.swarmingUser, agent.swarmingBotDir); err != nil {
		return err
	}

	path := filepath.Join(agent.swarmingBotDir, "swarming_bot.zip")
	if err := agent.downloadSwarmingBotCode(ctx, serviceAccount, server, path); err != nil {
		return err
	}

	substitutions := struct {
		Path string
		User string
	}{
		Path: path,
		User: agent.swarmingUser,
	}
	content, err := substitute(ctx, string(GetAsset(agent.swarmingAutoStartTemplate)), substitutions)
	if err != nil {
		return err
	}
	path, err = substitute(ctx, agent.swarmingAutoStartPath, substitutions)
	if err != nil {
		return err
	}
	_, err = os.Stat(path)
	if err == nil || os.IsExist(err) {
		logging.Infof(ctx, "Reinstalling: %s.", path)
	} else {
		logging.Infof(ctx, "Installing: %s.", path)
	}
	return ioutil.WriteFile(path, []byte(content), 0644)
}

// Downloads the Swarming bot code.
func (agent *Agent) downloadSwarmingBotCode(ctx context.Context, serviceAccount string, server string, path string) error {
	_, err := os.Stat(path)
	if err == nil || os.IsExist(err) {
		logging.Infof(ctx, "Already installed: %s.", path)
		return nil
	}

	logging.Infof(ctx, "Installing: %s.", path)
	options := auth.Options{
		GCEAccountName: serviceAccount,
		Method:         auth.GCEMetadataMethod,
	}
	client, err := auth.NewAuthenticator(ctx, auth.SilentLogin, options).Client()
	if err != nil {
		return err
	}
	response, err := client.Get(server + "/bot_code")
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		// TODO(smut): Differentiate between transient and non-transient.
		return errors.New("unexpected HTTP status: " + response.Status)
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, response.Body)
	if err != nil {
		return err
	}
	return agent.strategy.chown(ctx, agent.swarmingUser, path)
}

// Installs the agent and configures auto-start.
func (agent *Agent) install(ctx context.Context) error {
	exe, err := osext.Executable()
	if err != nil {
		return err
	}

	substitutions := struct {
		Agent string
		User  string
	}{
		Agent: exe,
		User:  agent.swarmingUser,
	}
	content, err := substitute(ctx, string(GetAsset(agent.agentAutoStartTemplate)), substitutions)
	if err != nil {
		return err
	}
	path, err := substitute(ctx, agent.agentAutoStartPath, substitutions)
	if err != nil {
		return err
	}

	_, err = os.Stat(path)
	if err == nil || os.IsExist(err) {
		bytes, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		logging.Infof(ctx, "%s", bytes)
		logging.Infof(ctx, "%s", content)
		if string(bytes) == content {
			logging.Infof(ctx, "Already installed: %s.", path)
			return nil
		}
		logging.Infof(ctx, "Reinstalling: %s.", path)
	} else {
		logging.Infof(ctx, "Installing: %s.", path)
	}
	return ioutil.WriteFile(path, []byte(content), 0644)
}

// Polls for instructions from Machine Provider.
//
// Does not return except in case of error.
func (agent *Agent) poll(ctx context.Context) error {
	// Metadata tells us which Machine Provider instance to talk to
	// and how to authenticate.
	hostname, err := metadata.InstanceName()
	if err != nil {
		return err
	}
	server, err := metadata.Get("instance/attributes/machine_provider_server")
	if err != nil {
		return err
	}
	serviceAccount, err := metadata.Get("instance/attributes/machine_service_account")
	if err != nil {
		return err
	}

	options := auth.Options{
		GCEAccountName:         serviceAccount,
		ServiceAccountJSONPath: auth.GCEServiceAccount,
	}
	client, err := auth.NewAuthenticator(ctx, auth.SilentLogin, options).Client()
	if err != nil {
		return err
	}
	mp := &MachineProvider {
		Client: client,
		Server: server,
	}

	for {
		logging.Infof(ctx, "Polling: %s.", server)
		instruction, err := mp.poll(ctx, hostname, "GCE")
		if err != nil {
			// Log error but don't return. Keep polling.
			logging.Errorf(ctx, "%s", err.Error())
		}
		if err = agent.handle(ctx, instruction, mp, hostname, serviceAccount); err != nil {
			return err
		}
		time.Sleep(60 * time.Second)
	}
}

// Handles a received instruction.
func (agent *Agent) handle(ctx context.Context, instruction *Instruction, mp *MachineProvider, hostname string, serviceAccount string) error {
	if instruction.State == "" || instruction.State == "EXECUTED" {
		return nil
	}

	logging.Infof(ctx, "Received new instruction:\n%s", instruction)
	// The only type of instruction that exists is to connect to Swarming.
	if err := agent.configureSwarmingAutoStart(ctx, serviceAccount, instruction.Instruction.SwarmingServer); err != nil {
		return err
	}
	if err := mp.ack(ctx, hostname, "GCE"); err != nil {
		return err
	}
	return agent.reboot(ctx)
}

// Reboot.
//
// Reboots the machine.
func (agent *Agent) reboot(ctx context.Context) error {
	logging.Infof(ctx, "Rebooting.")
	for {
		if err := agent.strategy.reboot(ctx); err != nil {
			return err
		}
		time.Sleep(60 * time.Second)
		logging.Infof(ctx, "Waiting to reboot...")
	}
}

func Main(args []string) int {
	var agent *Agent
	var err error

	var install bool
	var user string
	flag.BoolVar(&install, "install", false, "Install the agent and exit.")
	flag.StringVar(&user, "user", "chrome-bot", "User to set up Swarming for.")
	flag.Parse()

	// Set up context and install the command line logger.
	// Platform-specific agents will set up logging to a file.
	ctx := context.Background()
	config := gologger.LoggerConfig{
		Format: gologger.StdFormatWithColor,
		Out:    os.Stderr,
	}
	ctx = config.Use(ctx)

	// Determine the platform-specific agent to use.
	agent, err = getAgent(ctx)
	if err != nil {
		logging.Errorf(ctx, "%s", err.Error())
		return 1
	}
	agent.swarmingUser = user

	ctx, err = agent.configureLogging(ctx)
	if err != nil {
		logging.Errorf(ctx, "%s", err.Error())
		return 1
	}

	if install {
		if err = agent.install(ctx); err != nil {
			logging.Errorf(ctx, "%s", err.Error())
			return 1
		}
		return 0
	}

	if err = agent.poll(ctx); err != nil {
		logging.Errorf(ctx, "%s", err.Error())
		return 1
	}
	return 0
}

func main() {
	mathrand.SeedRandomly()

	os.Exit(Main(os.Args[1:]))
}
