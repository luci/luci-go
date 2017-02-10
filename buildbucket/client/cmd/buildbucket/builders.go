// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strconv"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"

	"github.com/luci/luci-go/buildbucket/client/cmd/buildbucket/proto"
	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/data/text/indented"
)

var cmdConvertBuilders = &subcommands.Command{
	UsageLine: "convert-builders",
	ShortDesc: "converts builders.pyl to swarmbucket definitions",
	LongDesc:  "Reads builders.pyl file from stdin, converts them to swarmbucket definitions and prints to stdout",
	Advanced:  true,
	CommandRun: func() subcommands.CommandRun {
		r := &convertBuildersRun{}
		r.Flags.StringVar(&r.recipeRepo,
			"recipe-repository",
			"https://chromium.googlesource.com/chromium/tools/build",
			"repository that contains recipes")
		r.Flags.StringVar(&r.builderSuffix, "builder-suffix", " (Swarming)", "builder name suffix")
		r.Flags.StringVar(&r.poolDim, "pool", "Chrome", "pool dimension to use")
		return r
	},
}

type convertBuildersRun struct {
	baseCommandRun
	recipeRepo    string
	builderSuffix string
	poolDim       string
}

func (r *convertBuildersRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if len(args) > 1 {
		return r.done(ctx, fmt.Errorf("unexpected arguments; pipe builders.pyl file into my stdin"))
	}
	if err := r.convertStreams(os.Stdin, os.Stderr); err != nil {
		return r.done(ctx, err)
	}
	return 0
}

func (r *convertBuildersRun) convertStreams(in io.Reader, out io.Writer) error {
	var input buildersFile
	if err := parsePyl(in, &input); err != nil {
		return fmt.Errorf("could not parse stdin: %s", err)
	}

	output, err := r.convert(&input)
	if err != nil {
		return err
	}
	return writeConfig(out, output)
}

func (r *convertBuildersRun) convert(in *buildersFile) (*buildbucket.Swarming, error) {
	if len(in.Builders) == 0 {
		return nil, fmt.Errorf("no builders")
	}

	commonDims, err := in.commonDimensions()
	if err != nil {
		return nil, err
	}
	commonDims["pool"] = r.poolDim
	commonProps := in.commonProperties()
	commonProperties, commonPropertiesJ := encodeProperties(commonProps)
	out := &buildbucket.Swarming{
		BuilderDefaults: &buildbucket.Swarming_Builder{
			Recipe: &buildbucket.Swarming_Recipe{
				Name:        proto.String(in.commonRecipe()),
				Properties:  commonProperties,
				PropertiesJ: commonPropertiesJ,
				Repository:  &r.recipeRepo,
			},
			Dimensions:   mapToList(commonDims),
			SwarmingTags: []string{"allow_milo:1"},
		},
	}

	for name, b := range in.Builders {
		pool, err := in.pool(b)
		if err != nil {
			panic(err)
		}
		dims, err := pool.dimensions()
		if err != nil {
			panic(err)
		}
		dims = mapDiff(dims, commonDims)
		delete(dims, "pool")

		props := mapDiff(b.Properties, commonProps)
		properties, propertiesJ := encodeProperties(props)
		recipeName := b.Recipe
		if recipeName == out.BuilderDefaults.Recipe.GetName() {
			recipeName = ""
		}
		out.Builders = append(out.Builders, &buildbucket.Swarming_Builder{
			Category:             &b.Category,
			Dimensions:           mapToList(dims),
			ExecutionTimeoutSecs: proto.Uint32(uint32(b.ExecutionTimeoutSecs)),
			Name:                 proto.String(name + r.builderSuffix),
			Recipe: &buildbucket.Swarming_Recipe{
				Name:        &recipeName,
				Properties:  properties,
				PropertiesJ: propertiesJ,
			},
		})
	}

	sort.Sort(byCategoryThenName(out.Builders))
	return out, nil
}

var pylToJSONScript = `
import ast, json, sys
d = ast.literal_eval(sys.stdin.read())
json.dump(d, sys.stdout)
`

func parsePyl(r io.Reader, v interface{}) error {
	cmd := exec.Command("python", "-u", "-c", pylToJSONScript)
	cmd.Stdin = r
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not parse pyl format: %s. Output: %s", err, out)
	}
	err = json.Unmarshal(out, v)
	if err != nil {
		err = fmt.Errorf("could not convert PYL to JSON: %s", err)
	}
	return err
}

// buildersFile is a structure for portions of builders.pyl file that we need.
// The format is documented in
// https://chromium.googlesource.com/infra/infra/+/ce976a0130570d5a66891bb8671f0b73ffc95c4d/doc/users/services/buildbot/builders.pyl.md
type buildersFile struct {
	Builders   map[string]*builder   `json:"builders"`
	SlavePools map[string]*slavePool `json:"slave_pools"`
}
type builder struct {
	Category             string                 `json:"category"`
	Recipe               string                 `json:"recipe"`
	Properties           map[string]interface{} `json:"properties"`
	SlavePoolNames       []string               `json:"slave_pools"`
	ExecutionTimeoutSecs intOrString            `json:"builder_timeout_s"`
}
type slavePool struct {
	slaveData `json:"slave_data"`
	// there is also "slaves" property, but we don't need it.
}
type slaveData struct {
	Bitness   intOrString `json:"bits"`
	OS        string      `json:"os"`
	OSVersion string      `json:"version"`
}

// intOrString is an integer that supports JSON unmarshaling from a string
type intOrString int

// UnmarshalJSON unmarshals data into n.
// data is expected to be a JSON string. If the string
// fails to parse to an integer, UnmarshalJSON returns
// an error.
func (n *intOrString) UnmarshalJSON(data []byte) error {
	data = bytes.Trim(data, `"`)
	num, err := strconv.Atoi(string(data))
	if err != nil {
		return err
	}
	*n = intOrString(num)
	return nil
}

func (f *buildersFile) pool(b *builder) (*slavePool, error) {
	if len(b.SlavePoolNames) != 1 {
		return nil, fmt.Errorf("builders with multiple slave pools are not supported")
	}
	slavePool := f.SlavePools[b.SlavePoolNames[0]]
	if slavePool == nil {
		return nil, fmt.Errorf("slave pool %q is not found", slavePool)
	}
	return slavePool, nil
}

func (f *buildersFile) commonProperties() map[string]interface{} {
	props := make([]map[string]interface{}, 0, len(f.Builders))
	for _, b := range f.Builders {
		props = append(props, b.Properties)
	}
	return frequentValues(props)
}

func (f *buildersFile) commonRecipe() string {
	counts := make(map[interface{}]int, len(f.Builders))
	for _, b := range f.Builders {
		counts[b.Recipe]++
	}

	if result, ok := frequentValue(counts); ok {
		return result.(string)
	}
	return ""
}

func (f *buildersFile) commonDimensions() (map[string]interface{}, error) {
	allDims := make([]map[string]interface{}, 0, len(f.Builders))
	for name, b := range f.Builders {
		pool, err := f.pool(b)
		if err != nil {
			return nil, fmt.Errorf("builder %q: %s", name, err)
		}
		dims, err := pool.dimensions()
		if err != nil {
			return nil, fmt.Errorf("builder %q: %s", name, err)
		}
		allDims = append(allDims, dims)
	}

	return frequentValues(allDims), nil
}

func (s *slavePool) dimensions() (map[string]interface{}, error) {
	var os string
	switch s.OS {
	case "linux":
		switch s.OSVersion {
		case "":
			os = "Linux"
		case "precise":
			os = "Ubuntu-12.04"
		case "trusty":
			os = "Ubuntu-14.04"
		}

	case "mac":
		switch s.OSVersion {
		case "":
			os = "Mac"
		case "10.9", "10.10", "10.11":
			os = "Mac-" + s.OSVersion
		}

	case "win":
		switch s.OSVersion {
		case "":
			os = "Windows"
		case "win7":
			os = "Windows-7-SP1"
		case "win8":
			os = "Windows-8.1-SP0"
		case "win10":
			os = "Windows-10-10586"
		case "2008":
			os = "Windows-2008ServerR2-SP1"
		}
	}
	if os == "" {
		return nil, fmt.Errorf("unsupported OS %q version %q", s.OS, s.OSVersion)
	}

	var cpu string
	switch s.Bitness {
	case 0:
		cpu = "x86"
	case 32:
		cpu = "x86-32"
	case 64:
		cpu = "x86-64"
	default:
		return nil, fmt.Errorf("unsupported bitness: %d", s.Bitness)
	}

	return map[string]interface{}{
		"os":  os,
		"cpu": cpu,
	}, nil
}

func encodeProperties(props map[string]interface{}) (properties, propertiesJ []string) {
	for k, v := range props {
		if s, ok := v.(string); ok {
			properties = append(properties, fmt.Sprintf("%s:%s", k, s))
		} else {
			marshaled, err := json.Marshal(v)
			if err != nil {
				panic(err)
			}
			propertiesJ = append(propertiesJ, fmt.Sprintf("%s:%s", k, marshaled))
		}
	}
	sort.Strings(properties)
	sort.Strings(propertiesJ)
	return
}

func writeConfig(w io.Writer, cfg *buildbucket.Swarming) error {
	indented := &indented.Writer{Writer: w, UseSpaces: true}

	var writeError error
	p := func(f string, a ...interface{}) {
		if writeError == nil {
			_, writeError = fmt.Fprintf(indented, f, a...)
			fmt.Fprint(indented, "\n")
		}
	}
	pstr := func(field, value string) {
		if value != "" {
			p("%s: %q", field, value)
		}
	}
	pstrlist := func(field string, values []string) {
		for _, v := range values {
			pstr(field, v)
		}
	}

	printRecipe := func(r *buildbucket.Swarming_Recipe) {
		pstr("repository", r.GetRepository())
		pstr("name", r.GetName())

		var properties []string
		fieldName := make(map[string]string, len(r.Properties)+len(r.PropertiesJ))
		for _, p := range r.Properties {
			properties = append(properties, p)
			fieldName[p] = "properties"
		}
		for _, p := range r.PropertiesJ {
			properties = append(properties, p)
			fieldName[p] = "properties_j"
		}
		sort.Strings(properties)
		for _, prop := range properties {
			pstr(fieldName[prop], prop)
		}
	}

	printBuilder := func(b *buildbucket.Swarming_Builder) {
		pstr("category", b.GetCategory())
		pstr("name", b.GetName())
		pstrlist("swarming_tags", b.SwarmingTags)
		pstrlist("dimensions", b.Dimensions)
		if b.GetExecutionTimeoutSecs() > 0 {
			p("execution_timeout_secs: %d", b.GetExecutionTimeoutSecs())
		}

		if b.Recipe.GetRepository() != "" || b.Recipe.GetName() != "" ||
			len(b.Recipe.Properties) > 0 || len(b.Recipe.PropertiesJ) > 0 {
			p("recipe {")
			indented.Level += 2
			printRecipe(b.Recipe)
			indented.Level -= 2
			p("}")
		}
	}

	p("builder_defaults {")
	indented.Level += 2
	printBuilder(cfg.BuilderDefaults)
	indented.Level -= 2
	p("}")

	p("\n# Keep builders sorted by category, then name.")
	builders := make(byCategoryThenName, len(cfg.Builders))
	copy(builders, cfg.Builders)
	sort.Sort(builders)

	for _, b := range builders {
		p("\nbuilders {")
		indented.Level += 2
		printBuilder(b)
		indented.Level -= 2
		p("}")
	}

	return writeError
}

// freqValues returns frequently used values
func frequentValues(maps []map[string]interface{}) map[string]interface{} {
	freq := map[string]map[interface{}]int{}

	// find all keys
	for _, m := range maps {
		for k := range m {
			if freq[k] == nil {
				freq[k] = make(map[interface{}]int, len(maps))
			}
		}
	}

	// for each key count occurrences of values, including unset values.
	for k, counts := range freq {
		for _, m := range maps {
			counts[m[k]]++
		}
	}

	result := map[string]interface{}{}
	for k, counts := range freq {
		if v, ok := frequentValue(counts); ok && v != nil {
			result[k] = v
		}
	}
	return result
}

func frequentValue(counts map[interface{}]int) (interface{}, bool) {
	total := 0
	for _, c := range counts {
		total += c
	}
	for v, c := range counts {
		fraction := float64(c) / float64(total)
		if fraction >= 0.7 {
			return v, true
		}
	}
	return nil, false
}

func mapToList(m map[string]interface{}) []string {
	var result []string
	for k, v := range m {
		result = append(result, fmt.Sprintf("%s:%s", k, v))
	}
	sort.Strings(result)
	return result
}

// mapDiff returns a map with values different from defaults.
func mapDiff(values, defaults map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(values))

	// include values that are different from defaults
	for k, v := range values {
		if dv, ok := defaults[k]; !ok || !reflect.DeepEqual(v, dv) {
			result[k] = v
		}
	}

	// negate defaults that were not explicitly specified in values
	for k, dv := range defaults {
		zero := reflect.Zero(reflect.TypeOf(dv)).Interface()
		if _, ok := values[k]; !ok && !reflect.DeepEqual(zero, dv) {
			result[k] = zero
		}
	}
	return result
}

type byCategoryThenName []*buildbucket.Swarming_Builder

func (a byCategoryThenName) Len() int      { return len(a) }
func (a byCategoryThenName) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a byCategoryThenName) Less(i, j int) bool {
	switch {
	case a[i].GetCategory() < a[j].GetCategory():
		return true

	case a[i].GetCategory() > a[j].GetCategory():
		return false

	default:
		return a[i].GetName() < a[j].GetName()
	}
}
