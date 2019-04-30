package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gce/api/config/v1"
)

type Config struct {
	VMs   config.Configs
	Kinds config.Kinds
}

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("expecting 1 positional argument: path to vms.cfg")
	}

	vmsPath := os.Args[1]
	kindsPath := filepath.Join(filepath.Dir(vmsPath), "kinds.cfg")

	cfg := Config{}
	readProto(vmsPath, &cfg.VMs)
	readProto(kindsPath, &cfg.Kinds)

	if err := merge(&cfg); err != nil {
		log.Fatalf("failed to merge - %s", err)
	}
	if err := normalize(&cfg); err != nil {
		log.Fatalf("failed to normalize - %s", err)
	}
	fmt.Println(proto.MarshalTextString(&cfg.VMs))
}

func readProto(path string, msg proto.Message) {
	blob, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatalf("failed to read %s: %s", path, err)
	}
	if err := proto.UnmarshalText(string(blob), msg); err != nil {
		log.Fatalf("failed to parse %s: %s", path, err)
	}
}

// Mostly copied from gce/appengine/config/config.go

func merge(cfg *Config) error {
	kindsMap := cfg.Kinds.Map()
	for _, v := range cfg.VMs.GetVms() {
		if v.Kind != "" {
			k, ok := kindsMap[v.Kind]
			if !ok {
				return errors.Reason("unknown kind %q", v.Kind).Err()
			}
			// Merge the config's attributes into a copy of the kind's.
			// This ensures the config's attributes overwrite the kind's.
			attrs := proto.Clone(k.Attributes).(*config.VM)
			// By default, proto.Merge concatenates repeated field values.
			// Instead, make repeated fields in the config override the kind.
			if len(v.Attributes.Disk) > 0 {
				attrs.Disk = nil
			}
			if len(v.Attributes.Metadata) > 0 {
				attrs.Metadata = nil
			}
			if len(v.Attributes.NetworkInterface) > 0 {
				attrs.NetworkInterface = nil
			}
			if len(v.Attributes.Tag) > 0 {
				attrs.Tag = nil
			}
			proto.Merge(attrs, v.Attributes)
			v.Attributes = attrs
			v.Kind = "" // expanded it already
		}
	}
	return nil
}

func normalize(cfg *Config) error {
	for _, v := range cfg.VMs.GetVms() {
		for _, ch := range v.Amount.GetChange() {
			if err := ch.Length.Normalize(); err != nil {
				return errors.Annotate(err, "failed to normalize %q", v.Prefix).Err()
			}
		}
		if err := v.Lifetime.Normalize(); err != nil {
			return errors.Annotate(err, "failed to normalize %q", v.Prefix).Err()
		}
		if err := v.Timeout.Normalize(); err != nil {
			return errors.Annotate(err, "failed to normalize %q", v.Prefix).Err()
		}
		v.Attributes.SetZone(v.Attributes.Zone)
	}
	sort.Slice(cfg.VMs.Vms, func(i, j int) bool {
		return cfg.VMs.Vms[i].Prefix < cfg.VMs.Vms[j].Prefix
	})
	return nil
}
