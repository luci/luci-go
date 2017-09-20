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
	"encoding/json"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"cloud.google.com/go/compute/metadata"

	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/logging/teelogger"

	"golang.org/x/net/context"
)

type Strategy interface {
	// Changes ownership of a path.
	chown(ctx context.Context, username string, path string) error
	// Starts the agent.
	start(ctx context.Context, path string) error
	// Stops all instances of the agent.
	stop(ctx context.Context) error
	// Reboots the machine.
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
	err := os.MkdirAll(agent.logsDir, 0755)
	if (err != nil) {
		return ctx, err
	}

	// TODO(smut): Capture logging emitted before configureLogging was called and
	// write it to the log file.
	out, err := os.OpenFile(filepath.Join(agent.logsDir, "agent.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if (err != nil) {
		return ctx, err
	}
	config := gologger.LoggerConfig { Out: out }
	return teelogger.Use(ctx, config.NewLogger), nil
}

// Configures auto-connect to the given Swarming server on reboot.
func (agent *Agent) configureSwarmingAutoStart(ctx context.Context, serviceAccount string, server string) error {
	os.MkdirAll(agent.swarmingBotDir, 0755)
	agent.strategy.chown(ctx, agent.swarmingUser, agent.swarmingBotDir)

	path := filepath.Join(agent.swarmingBotDir, "swarming_bot.zip")
	agent.downloadSwarmingBotCode(ctx, serviceAccount, server, path)

	substitutions := struct {
		Path string
		User string
	}{
		Path: path,
		User: agent.swarmingUser,
	}
	content, err := substituteFile(ctx, agent.swarmingAutoStartTemplate, substitutions)
	if (err != nil) {
		return err
	}
	path, err = substitute(ctx, agent.swarmingAutoStartPath, substitutions)
	if (err != nil) {
		return err
	}
	_, err = os.Stat(path)
	if (err == nil || os.IsExist(err)) {
		logging.Infof(ctx, "Reinstalling: %s.", path)
	} else {
		logging.Infof(ctx, "Installing: %s.", path)
	}
	return ioutil.WriteFile(path, []byte(content), 0644)
}

// Downloads the Swarming bot code.
func (agent *Agent) downloadSwarmingBotCode(ctx context.Context, serviceAccount string, server string, path string) error {
	_, err := os.Stat(path)
	if (err == nil || os.IsExist(err)) {
		logging.Infof(ctx, "Already installed: %s.", path)
		return nil
	}

	logging.Infof(ctx, "Installing: %s.", path)
	options := auth.Options{
		GCEAccountName: serviceAccount,
		ServiceAccountJSONPath: auth.GCEServiceAccount,
	}
	httpClient, err := auth.NewAuthenticator(ctx, auth.SilentLogin, options).Client()
	if (err != nil) {
		return err
	}
	response, err := httpClient.Get(server + "/bot_code")
	if (err != nil) {
		return err
	}
	defer response.Body.Close()
	if (response.Status != "200 OK") {
		// TODO(smut): Differentiate between transient and non-transient.
		return errors.New("Unexpected HTTP status: " + response.Status + ".")
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if (err != nil) {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, response.Body)
	if (err != nil) {
		return err
	}
	return agent.strategy.chown(ctx, agent.swarmingUser, path)
}

// Installs the agent and configures auto-start.
func (agent *Agent) install(ctx context.Context) error {
	substitutions := struct {
		Agent string
		User string
	}{
		Agent: os.Args[0],
		User: agent.swarmingUser,
	}
	content, err := substituteFile(ctx, agent.agentAutoStartTemplate, substitutions)
	if (err != nil) {
		return err
	}
	path, err := substitute(ctx, agent.agentAutoStartPath, substitutions)
	if (err != nil) {
		return err
	}

	_, err = os.Stat(path)
	if (err == nil || os.IsExist(err)) {
		bytes, err := ioutil.ReadFile(path)
		if (err != nil) {
			return err
		}
		logging.Infof(ctx, string(bytes))
		logging.Infof(ctx, content)
		if (string(bytes) == content) {
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
	if (err != nil) {
		return err
	}
	server, err := metadata.Get("instance/attributes/machine_provider_server")
	if (err != nil) {
		return err
	}
	serviceAccount, err := metadata.Get("instance/attributes/machine_service_account")
	if (err != nil) {
		return err
	}

	ackUrl := server + "/_ah/api/machine/v1/ack"
	pollUrl := server + "/_ah/api/machine/v1/poll"
	body, err := json.Marshal(struct {
		Backend string	`json:"backend"`
		Hostname string	`json:"hostname"`
	}{
		Backend: "GCE",
		Hostname: hostname,
	})
	if (err != nil) {
		return err
	}
	options := auth.Options{
		GCEAccountName: serviceAccount,
		ServiceAccountJSONPath: auth.GCEServiceAccount,
	}
	httpClient, err := auth.NewAuthenticator(ctx, auth.SilentLogin, options).Client()
	if (err != nil) {
		return err
	}

	for {
		logging.Infof(ctx, "Polling: %s.", server)
		instruction, err := poll(ctx, httpClient, pollUrl, body)
		if (err != nil) {
			logging.Errorf(ctx, err.Error())
		} else if (instruction.State != "EXECUTED") {
			logging.Infof(ctx, "Received new instruction.\n%s", instruction)
			if (instruction.Instruction.SwarmingServer != "") {
				err = agent.configureSwarmingAutoStart(ctx, serviceAccount, instruction.Instruction.SwarmingServer)
				if (err != nil) {
					logging.Errorf(ctx, err.Error())
				} else {
					err = ack(ctx, httpClient, ackUrl, body)
					if (err != nil) {
						logging.Errorf(ctx, err.Error())
					} else {
						err = agent.reboot(ctx)
						if (err != nil) {
							logging.Errorf(ctx, err.Error())
						}
					}
				}
			}
		}
		time.Sleep(60 * time.Second)
	}
}

// Reboot.
//
// Reboots the machine.
func (agent *Agent) reboot(ctx context.Context) error {
	logging.Infof(ctx, "Rebooting.")
	for {
		err := agent.strategy.reboot(ctx)
		if (err != nil) {
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
	config := gologger.LoggerConfig {
		Format: gologger.StdFormatWithColor,
		Out: os.Stderr,
	}
	ctx = config.Use(ctx)

	// Determine the platform-specific agent to use.
	if (runtime.GOOS == "linux") {
		agent, err = getAgent(ctx)
		if (err != nil) {
			logging.Errorf(ctx, err.Error())
			return 1
		}
	} else if (runtime.GOOS == "windows") {
		agent, err = getAgent(ctx)
		if (err != nil) {
			logging.Errorf(ctx, err.Error())
			return 1
		}
	} else {
		logging.Errorf(ctx, "Unsupported operating system: %s.", runtime.GOOS)
		return 1
	}

	agent.swarmingUser = user

	ctx, err = agent.configureLogging(ctx)
	if (err != nil) {
		logging.Errorf(ctx, err.Error())
		return 1
	}

	if (install) {
		err = agent.install(ctx)
		if (err != nil) {
			logging.Errorf(ctx, err.Error())
			return 1
		}
		return 0
	}

	err = agent.poll(ctx)
	if (err != nil) {
		logging.Errorf(ctx, err.Error())
		return 1
	}
	return 0
}

func main() {
	mathrand.SeedRandomly()

	os.Exit(Main(os.Args[1:]))
}
