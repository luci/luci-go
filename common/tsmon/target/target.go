// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package target contains information about the thing that is sending metrics -
// either a NetworkDevice (a machine) or a Task (a service).
package target

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"

	"github.com/golang/protobuf/proto"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto"
)

var (
	targetType = flag.String("ts-mon-target-type", "device",
		"the type of target that is being monitored ('device' or 'task')")
	deviceHostname = flag.String("ts-mon-device-hostname", "",
		"name of this device")
	deviceRegion = flag.String("ts-mon-device-region", "",
		"name of the region this devices lives in")
	deviceRole = flag.String("ts-mon-device-role", "default",
		"role of the device")
	deviceNetwork = flag.String("ts-mon-device-network", "",
		"name of the network this device is connected to")
	taskServiceName = flag.String("ts-mon-task-service-name", "",
		"name of the service being monitored")
	taskJobName = flag.String("ts-mon-task-job-name", "",
		"name of this job instance of the task")
	taskRegion = flag.String("ts-mon-task-region", "",
		"name of the region in which this task is running")
	taskHostname = flag.String("ts-mon-task-hostname", "",
		"name of the host on which this task is running")
	taskNumber = flag.Int("ts-mon-task-number", 0,
		"number (e.g. for replication) of this instance of this task")
)

// A Target knows how to put information about itself in a MetricsData message.
type Target interface {
	PopulateProto(d *pb.MetricsData)
}

// A Task is a process or a service running on one or more machine.
type Task pb.Task

// AsProto returns this object as a tsmon.proto.Task message.
func (t *Task) AsProto() *pb.Task { return (*pb.Task)(t) }

// PopulateProto implements Target.
func (t *Task) PopulateProto(d *pb.MetricsData) { d.Task = t.AsProto() }

// A NetworkDevice is a machine that has a hostname.
type NetworkDevice pb.NetworkDevice

// AsProto returns this object as a tsmon.proto.NetworkDevice message.
func (t *NetworkDevice) AsProto() *pb.NetworkDevice { return (*pb.NetworkDevice)(t) }

// PopulateProto implements Target.
func (t *NetworkDevice) PopulateProto(d *pb.MetricsData) { d.NetworkDevice = t.AsProto() }

// NewFromFlags returns a Target configured from commandline flags.
func NewFromFlags() (Target, error) {
	if *targetType == "task" {
		if *taskServiceName == "" {
			return nil, errors.New(
				"--ts-mon-task-service-name must be provided when using --ts-mon-target-type=task")
		}
		if *taskJobName == "" {
			return nil, errors.New(
				"--ts-mon-task-job-name must be provided when using --ts-mon-target-type=task")
		}

		return (*Task)(&pb.Task{
			ServiceName: proto.String(*taskServiceName),
			JobName:     proto.String(*taskJobName),
			DataCenter:  proto.String(*taskRegion),
			HostName:    proto.String(*taskHostname),
			TaskNum:     proto.Int32(int32(*taskNumber)),
		}), nil
	} else if *targetType == "device" {
		return (*NetworkDevice)(&pb.NetworkDevice{
			Alertable: proto.Bool(true),
			Realm:     proto.String("ACQ_CHROME"),
			Metro:     proto.String(*deviceRegion),
			Role:      proto.String(*deviceRole),
			Hostname:  proto.String(*deviceHostname),
			Hostgroup: proto.String(*deviceNetwork),
		}), nil
	} else {
		return nil, fmt.Errorf("unknown --ts-mon-target-type '%s'", *targetType)
	}
}

func init() {
	hostname, region := getFQDN()

	setFlagDefault("ts-mon-device-hostname", hostname)
	setFlagDefault("ts-mon-task-hostname", hostname)

	setFlagDefault("ts-mon-device-region", region)
	setFlagDefault("ts-mon-task-region", region)

	if network, ok := getNetwork(hostname); ok {
		setFlagDefault("ts-mon-device-network", network)
	}
}

func setFlagDefault(name string, value string) {
	f := flag.Lookup(name)
	f.Value.Set(value)
	f.DefValue = value
}

func getFQDN() (string, string) {
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				if names, err := net.LookupAddr(ipNet.IP.String()); err == nil {
					for _, name := range names {
						parts := strings.Split(name, ".")
						if len(parts) > 1 {
							return strings.ToLower(parts[0]), strings.ToLower(parts[1])
						}
					}
				}
			}
		}
	}
	if hostname, err := os.Hostname(); err != nil {
		return strings.ToLower(hostname), "unknown"
	}
	return "unknown", "unknown"
}

func getNetwork(hostname string) (network string, ok bool) {
	if m := regexp.MustCompile(`^([\w-]*?-[acm]|master)(\d+)a?$`).FindStringSubmatch(hostname); m != nil {
		return m[2], true
	}
	return "", false
}
