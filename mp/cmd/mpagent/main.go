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
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"cloud.google.com/go/compute/metadata"

	"go.chromium.org/luci/auth"
	machine "go.chromium.org/luci/common/api/machine_provider/machine/v1"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/logging/teelogger"
	"go.chromium.org/luci/common/tsmon"
)

// Strategy is platform-specific agent implementation. See agent_*.go.
type Strategy interface {
	// chown changes ownership of a path.
	chown(ctx context.Context, username, path string) error
	// enableSwarming enables installed service.
	enableSwarming(ctx context.Context) error
	// start starts the agent.
	start(ctx context.Context, path string) error
	// stop stops all instances of the agent.
	stop(ctx context.Context) error
	// reboot reboots the machine.
	reboot(ctx context.Context) error
}

type Agent struct {
	// agentAutoStartPath is the path to install this agent's auto-start config to.
	agentAutoStartPath string
	// agentAutoStartTemplate is the name of the asset containing this agent's auto-start template.
	agentAutoStartTemplate string
	// client is the http.Client to use for HTTP requests.
	client *http.Client
	// hostname is the name of the host the agent is running on.
	hostname string
	// logsDir is the path to the directory where this agent should emit logging output.
	logsDir string
	// mp is the client to use to communicate with the Machine Provider server.
	mp *MachineProvider
	// server is the Machine Provider server to communicate with.
	server string
	// serviceAccount is the name of the service account to authenticate HTTP requests with.
	serviceAccount string
	// swarmingAutoStartPath is the path to install this agent's Swarming auto-start config to.
	swarmingAutoStartPath string
	// swarmingAutoStartTemplate is the name of the asset containing this agent's Swarming auto-start template.
	swarmingAutoStartTemplate string
	// swarmingBotDir is the directory where this agent should install the Swarming bot process.
	swarmingBotDir string
	// swarmingUser is the name of the user the agent should install Swarming for.
	swarmingUser string
	// strategy is the platform-specific implementation for this agent.
	strategy Strategy
}

// configureLogging configures logging to a file, in addition to any other logging.
//
// Returns modified context.Context.
func (agent *Agent) configureLogging(ctx context.Context) (context.Context, error) {
	log := fmt.Sprintf("agent.%s.log", strconv.FormatInt(time.Now().Unix(), 10))
	if err := os.MkdirAll(agent.logsDir, 0755); err != nil {
		return ctx, err
	}

	// TODO(smut): Capture logging emitted before configureLogging was called and
	// write it to the log file.
	out, err := os.OpenFile(filepath.Join(agent.logsDir, log), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return ctx, err
	}
	config := gologger.LoggerConfig{Out: out}
	return teelogger.Use(ctx, config.NewLogger), nil
}

// configureMonitoring configures tsmon monitoring.
func (agent *Agent) configureMonitoring(ctx context.Context) error {
	flags := tsmon.NewFlags()
	flags.Flush = "manual"
	flags.Target.AutoGenHostname = true
	flags.Target.TargetType = "task"
	flags.Target.TaskJobName = "default"
	flags.Target.TaskServiceName = "mpagent"
	return tsmon.InitializeFromFlags(ctx, &flags)
}

// configureSwarmingAutoStart configures auto-connect to the given Swarming server on reboot.
func (agent *Agent) configureSwarmingAutoStart(ctx context.Context, server string) error {
	if err := os.MkdirAll(agent.swarmingBotDir, 0755); err != nil {
		return err
	}
	if err := agent.strategy.chown(ctx, agent.swarmingUser, agent.swarmingBotDir); err != nil {
		return err
	}

	path := filepath.Join(agent.swarmingBotDir, "swarming_bot.zip")
	if err := agent.downloadSwarmingBotCode(ctx, server, path); err != nil {
		return err
	}

	substitutions := map[string]string{
		"Path": path,
		"User": agent.swarmingUser,
	}
	content, err := substituteAsset(ctx, agent.swarmingAutoStartTemplate, substitutions)
	if err != nil {
		return err
	}
	path, err = substitute(ctx, agent.swarmingAutoStartPath, substitutions)
	if err != nil {
		return err
	}
	_, err = os.Stat(path)
	switch {
	case err == nil:
		logging.Infof(ctx, "Reinstalling: %s.", path)
	case os.IsNotExist(err):
		logging.Infof(ctx, "Installing: %s.", path)
	default:
		return err
	}
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return agent.strategy.enableSwarming(ctx)
}

// downloadSwarminBotCode downloads the Swarming bot code from the given server.
func (agent *Agent) downloadSwarmingBotCode(ctx context.Context, server, path string) error {
	_, err := os.Stat(path)
	if err == nil {
		logging.Infof(ctx, "Already installed: %s.", path)
		return nil
	}

	logging.Infof(ctx, "Downloading: %s.", path)
	response, err := agent.client.Get(server + "/bot_code")
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

// initialize initializes the agent.
func (agent *Agent) initialize(ctx context.Context) (context.Context, error) {
	ctx, err := agent.configureLogging(ctx)
	if err != nil {
		return ctx, err
	}
	if err = agent.configureMonitoring(ctx); err != nil {
		return ctx, err
	}

	// TODO(smut): Remove fallback on metadata server.
	if agent.server == "" {
		agent.server, err = metadata.Get("instance/attributes/machine_provider_server")
		if err != nil {
			return ctx, err
		}
	}
	if agent.serviceAccount == "" {
		agent.serviceAccount, err = metadata.Get("instance/attributes/machine_service_account")
		if err != nil {
			return ctx, err
		}
	}

	options := auth.Options{
		GCEAccountName:         agent.serviceAccount,
		ServiceAccountJSONPath: auth.GCEServiceAccount,
	}
	agent.client, err = auth.NewAuthenticator(ctx, auth.SilentLogin, options).Client()
	if err != nil {
		return ctx, err
	}
	metadataClient := metadata.NewClient(agent.client)
	agent.hostname, err = metadataClient.InstanceName()
	if err != nil {
		return ctx, err
	}
	agent.mp, err = getClient(ctx, agent.client, agent.server)
	if err != nil {
		return ctx, err
	}
	return ctx, nil
}

// install installs the agent, starts it, and configures auto-start on reboot.
func (agent *Agent) install(ctx context.Context) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	substitutions := map[string]string{
		"Agent":          exe,
		"Server":         agent.server,
		"ServiceAccount": agent.serviceAccount,
		"User":           agent.swarmingUser,
	}
	content, err := substituteAsset(ctx, agent.agentAutoStartTemplate, substitutions)
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
		if string(bytes) == content {
			logging.Infof(ctx, "Already installed: %s.", path)
			return nil
		}
		logging.Infof(ctx, "Reinstalling: %s.", path)
		if err = agent.strategy.stop(ctx); err != nil {
			return err
		}
	} else {
		logging.Infof(ctx, "Installing: %s.", path)
	}
	if err = ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		return err
	}
	return agent.strategy.start(ctx, path)
}

// poll polls for instructions from Machine Provider.
//
// Does not return except in case of error.
func (agent *Agent) poll(ctx context.Context) error {
	// Metadata tells us which Machine Provider instance to talk to
	// and how to authenticate.
	for {
		logging.Infof(ctx, "Polling.")
		instruction, err := agent.mp.poll(ctx, agent.hostname, "GCE")
		if err != nil {
			// Log error but don't return. Keep polling.
			logging.Errorf(ctx, "%s", err.Error())
		} else {
			if err = agent.handle(ctx, instruction); err != nil {
				return err
			}
		}
		tsmon.Flush(ctx)
		time.Sleep(time.Minute)
	}
}

// handle handles a received instruction.
func (agent *Agent) handle(ctx context.Context, instruction *machine.ComponentsMachineProviderRpcMessagesPollResponse) error {
	if instruction.State == "" || instruction.State == "EXECUTED" {
		return nil
	}

	// The only type of instruction that exists is to connect to Swarming.
	if instruction.Instruction == nil || instruction.Instruction.SwarmingServer == "" {
		return nil
	}

	logging.Infof(ctx, "Received new instruction:\n%s", instruction)
	start := time.Now()
	if err := agent.configureSwarmingAutoStart(ctx, instruction.Instruction.SwarmingServer); err != nil {
		return err
	}
	if err := agent.mp.ack(ctx, agent.hostname, "GCE"); err != nil {
		return err
	}
	duration := float64(time.Since(start)) / float64(time.Second)
	instructionsAcked.Add(ctx, 1, agent.server, instruction.Instruction.SwarmingServer)
	instructionsAckedTime.Add(ctx, duration, agent.server, instruction.Instruction.SwarmingServer)
	return agent.reboot(ctx)
}

// reboot attempts to reboot the machine.
//
// Does not return except in case of error.
func (agent *Agent) reboot(ctx context.Context) error {
	logging.Infof(ctx, "Rebooting.")
	tsmon.Flush(ctx)
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
	var server, serviceAccount, user string
	flag.BoolVar(&install, "install", false, "Install the agent and exit.")
	flag.StringVar(&server, "server", "", "Machine Provider server URL.")
	flag.StringVar(&serviceAccount, "gce-service-account", "", "GCE service account name to authenticate with.")
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
	agent.server = server
	agent.serviceAccount = serviceAccount
	agent.swarmingUser = user

	ctx, err = agent.initialize(ctx)
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
