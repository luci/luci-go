// Copyright 2019 The LUCI Authors.
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

// Package generate implements 'semantic-diff' subcommand.
package diff

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"

	config_pb "go.chromium.org/luci/common/proto/config"
	cq_pb "go.chromium.org/luci/cq/api/config/v2"
	logdog_pb "go.chromium.org/luci/logdog/api/config/svcconfig"
	milo_pb "go.chromium.org/luci/milo/api/config"
	scheduler_pb "go.chromium.org/luci/scheduler/appengine/messages"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/lucicfg/cli/base"
)

// Cmd is 'semantic-diff' subcommand.
func Cmd(params base.Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "semantic-diff SCRIPT CONFIG [CONFIG CONFIG ...]",
		ShortDesc: "interprets a high-level config, compares the result to existing configs",
		LongDesc: `Interprets a high-level config, compares the result to existing configs.

THIS SUBCOMMAND WILL BE DELETED AFTER IT IS NO LONGER USEFUL. DO NOT DEPEND ON
IT IN ANY AUTOMATIC SCRIPTS. FOR MANUAL USE ONLY. IF YOU REALLY-REALLY NEED TO
USE IT FROM AUTOMATION, PLEASE FILE A BUG.

Uses semantic comparison. Normalizes all protos before comparing them via
'git diff'. Intended to be used manually when switching existing *.cfg to be
generated from *.star.

Accepts a path to the entry-point *.star script and paths to existing configs
to diff against. Their filenames (not full paths) will be used to find
corresponding generated files, and also to figure out the proto schema to use.

Example:

  $ lucicfg semantic-diff main.star configs/cr-buildbucket.cfg configs/luci-milo.cfg
`,
		CommandRun: func() subcommands.CommandRun {
			dr := &diffRun{}
			dr.Init(params)
			dr.AddMetaFlags()
			dr.Flags.StringVar(&dr.outputDir, "output-dir", "", "Where to put normalized configs if you want them preserved after the command completes.")
			return dr
		},
	}
}

type diffRun struct {
	base.Subcommand

	outputDir string
}

func (dr *diffRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !dr.CheckArgs(args, 2, -1) {
		return 1
	}

	if os.Getenv("SWARMING_HEADLESS") == "1" {
		fmt.Fprintf(os.Stderr, "Refusing to run 'semantic-diff' on a bot, this subcommand is supposed to be used only manually!\n")
		return 1
	}

	ctx := cli.GetContext(a, dr, env)
	err := dr.run(ctx, dr.outputDir, args[0], args[1:])
	return dr.Done(nil, err)
}

func (dr *diffRun) run(ctx context.Context, outputDir, inputFile string, cfgs []string) error {
	meta := dr.DefaultMeta()
	configSet, err := base.GenerateConfigs(ctx, inputFile, &meta, &dr.Meta)
	if err != nil {
		return err
	}

	logging.Infof(ctx, "Preparing configs for comparison...")

	// Discover all pairs of files we want to compare to each other.
	pairs := make([]*configPair, 0, len(cfgs))
	fail := false
	for _, path := range cfgs {
		blob, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			fail = true
			continue
		}

		pair := configPair{
			name:     filepath.Base(path),
			original: blob,
		}
		for name, body := range configSet {
			if strings.HasSuffix(name, pair.name) {
				pair.generated = body
				break
			}
		}
		if pair.generated == nil {
			fmt.Fprintf(os.Stderr, "No generated config file that matches %q\n", path)
			fail = true
			continue
		}

		for _, desc := range knownTypes {
			if strings.HasPrefix(pair.name, desc.prefix) {
				pair.typ = proto.MessageType(desc.proto)
				pair.protoNormalizer = desc.protoNormalizer
				pair.rawNormalizer = desc.rawNormalizer
				break
			}
		}
		if pair.typ == nil {
			fmt.Fprintf(os.Stderr, "Cannot guess proto type of %q\n", path)
			fail = true
			continue
		}

		pairs = append(pairs, &pair)
	}

	logging.Infof(ctx, "Normalizing configs...")
	for _, pair := range pairs {
		if err := pair.normalize(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to normalize %q: %s\n", pair.name, err)
			fail = true
		}
	}

	if fail {
		return fmt.Errorf("see the error log")
	}

	logging.Infof(ctx, "Diffing...")

	usingTemp := outputDir == ""
	if usingTemp {
		var err error
		outputDir, err = ioutil.TempDir("", "lucicfg")
		if err != nil {
			return err
		}
		defer os.RemoveAll(outputDir)
	}

	// Write normalize original files.
	old := filepath.Join(outputDir, "old")
	if err := os.MkdirAll(old, 0750); err != nil {
		return err
	}
	for _, pair := range pairs {
		if err := ioutil.WriteFile(filepath.Join(old, pair.name), pair.original, 0666); err != nil {
			return err
		}
	}

	// Write normalize generated files.
	new := filepath.Join(outputDir, "new")
	if err := os.MkdirAll(new, 0750); err != nil {
		return err
	}
	for _, pair := range pairs {
		if err := ioutil.WriteFile(filepath.Join(new, pair.name), pair.generated, 0666); err != nil {
			return err
		}
	}

	fmt.Printf("\nAbout to run:\n")
	fmt.Printf("git diff --no-index \\\n  %q \\\n  %q\n", old, new)
	if usingTemp {
		fmt.Printf("\nPass -output-dir flag to store the generated files in a separate directory " +
			"if you want to examine them later.\n")
	}

	// Ask git to diff them nicely.
	cmd := exec.Command("git", "diff", "--no-index", old, new)
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	switch err := cmd.Run().(type) {
	case nil:
		fmt.Printf("\nNo diff detected: the configs are semantically identical.\n")
		return nil
	case *exec.ExitError:
		return nil // a non-zero diff is fine
	default:
		return err // a failure to run diff is not fine
	}
}

////////////////////////////////////////////////////////////////////////////////

// protoNormalizer takes a proto message and converts it to normalized form,
// e.g. sorts entries, flattens mixins, etc.
type protoNormalizer func(context.Context, proto.Message) (proto.Message, error)

// rawNormalizer takes a byte blob with a config and normalizes it.
type rawNormalizer func(context.Context, []byte) ([]byte, error)

// A pair of config files of the same type to compare.
type configPair struct {
	name            string          // e.g. "cr-buildbucket.cfg"
	typ             reflect.Type    // e.g. &Config{}
	protoNormalizer protoNormalizer // callback to normalize the protos
	rawNormalizer   rawNormalizer   // callback to normalize raw []byte
	original        []byte          // body of the original file
	generated       []byte          // body of the generated file
}

// normalize normalizes both original and generated protos (in-place).
func (p *configPair) normalize(ctx context.Context) error {
	var err error
	p.original, err = normalizeOne(ctx, p.original, p)
	if err != nil {
		return fmt.Errorf("failed to normalize the original config - %s", err)
	}
	p.generated, err = normalizeOne(ctx, p.generated, p)
	if err != nil {
		return fmt.Errorf("failed to normalize the generated config - %s", err)
	}
	return nil
}

// normalizeOne deserializes the proto, passes it through normalizer, serializes
// it back.
func normalizeOne(ctx context.Context, in []byte, p *configPair) (out []byte, err error) {
	if p.rawNormalizer != nil {
		return p.rawNormalizer(ctx, in)
	}
	msg := reflect.New(p.typ.Elem()).Interface().(proto.Message)
	if err = proto.UnmarshalText(string(in), msg); err != nil {
		return
	}
	if msg, err = p.protoNormalizer(ctx, msg); err != nil {
		return
	}
	return []byte(proto.MarshalTextString(msg)), nil
}

////////////////////////////////////////////////////////////////////////////////

// TODO(vadimsh): Hardcoded prefixes is a hack.
var knownTypes = []struct {
	prefix          string
	proto           string
	protoNormalizer protoNormalizer
	rawNormalizer   rawNormalizer
}{
	{"commit-queue", "cq.config.Config", normCommitQueueCfg, nil},
	{"cr-buildbucket", "buildbucket.BuildbucketCfg", nil, normBuildbucketCfg},
	{"luci-logdog", "svcconfig.ProjectConfig", normLogdogCfg, nil},
	{"luci-milo", "milo.Project", normMiloCfg, nil},
	{"luci-scheduler", "scheduler.config.ProjectConfig", normSchedulerCfg, nil},
	{"project", "config.ProjectCfg", normProjectCfg, nil},
}

func normCommitQueueCfg(ctx context.Context, m proto.Message) (proto.Message, error) {
	pb := m.(*cq_pb.Config)
	// TODO
	return pb, nil
}

func normBuildbucketCfg(ctx context.Context, in []byte) (out []byte, err error) {
	// Install or update 'flatten_buildbucket_cfg' tool.
	bin, err := installFlattenBuildbucketCfg(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to install buildbucket config flattener: %s", err)
	}

	// 'flatten_buildbucket_cfg' wants a real file as input.
	f, err := ioutil.TempFile("", "lucicfg")
	if err != nil {
		return nil, err
	}
	defer func() {
		f.Close()
		os.Remove(f.Name())
	}()
	if _, err := f.Write(in); err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}

	buf := bytes.Buffer{}

	cmd := exec.Command(bin, f.Name())
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to flatten the config - %s", err)
	}

	return buf.Bytes(), nil
}

func normLogdogCfg(ctx context.Context, m proto.Message) (proto.Message, error) {
	pb := m.(*logdog_pb.ProjectConfig)
	// TODO
	return pb, nil
}

func normMiloCfg(ctx context.Context, m proto.Message) (proto.Message, error) {
	pb := m.(*milo_pb.Project)
	// TODO
	return pb, nil
}

func normSchedulerCfg(ctx context.Context, m proto.Message) (proto.Message, error) {
	pb := m.(*scheduler_pb.ProjectConfig)
	// TODO
	return pb, nil
}

func normProjectCfg(ctx context.Context, m proto.Message) (proto.Message, error) {
	pb := m.(*config_pb.ProjectCfg)
	// TODO
	return pb, nil
}

////////////////////////////////////////////////////////////////////////////////

var flattenerPath = ""

func installFlattenBuildbucketCfg(ctx context.Context) (bin string, err error) {
	// Do not install twice, just a waste of time resolving 'latest'.
	if flattenerPath != "" {
		return flattenerPath, nil
	}

	// Install into ~/.flatten_buildbucket_cfg because where else to keep random
	// garbage?
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(usr.HomeDir, ".flatten_buildbucket_cfg")
	logging.Infof(ctx, "Installing infra/tools/flatten_buildbucket_cfg into %s", dest)

	cmd := exec.Command("cipd", "ensure", "-root", dest, "-ensure-file", "-")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = strings.NewReader("infra/tools/flatten_buildbucket_cfg latest")
	if err = cmd.Run(); err != nil {
		return
	}

	flattenerPath = filepath.Join(dest, "flatten_buildbucket_cfg")
	return flattenerPath, nil
}
