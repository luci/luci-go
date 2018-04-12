package model

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"time"

	"go.chromium.org/gae/service/datastore"
)

type BuilderPool struct {
	// _id for a Pool is always 1
	_ int64 `gae:"$id,1"`

	// Parent is the ID for the BuilderSummary of the builder this pool is associated with.
	// This should always be a BuilderSummary.
	BuilderKey *datastore.Key `gae:"$parent"`

	// Dimensions represent the dimensions of a pool.
	Dimensions Dimensions

	// Machines is a slice of machines in the pool, along with its status.
	Machines MachinePool

	// LastUpdate is when this entity was last updated.
	LastUpdate time.Time
}

type Dimensions struct {
	// Host is the swarming hostname for these set of dimensions.
	Host string
	// Dimensions is a slice of strings in the format "key:value" representing the
	// set of dimensions for a builder.
	Dimensions []string
	// SHA1 is the SHA1 hash of the JSON serialized string of the dimensions.
	SHA1 string `json:"-"`
}

func NewDimensions(host string, dims []string) Dimensions {
	d := Dimensions{
		Dimensions: dims,
		Host:       host,
	}
	s, err := json.Marshal(d)
	if err != nil {
		// This should never happen.
		panic(err)
	}
	d.SHA1 = fmt.Sprintf("%x", sha1.Sum(s))
	return d
}

// MachinePool is a slice of machines.
type MachinePool []Machine

// Machine represents a single machine in a work pool.
type Machine struct {
	// Name is the short hostname of the Machine.
	Name string
	// Status is the current status of the Machine.
	Status MachineStatus
	// LastSeen denotes when the Machine was last seen.
	LastSeen time.Time
}
