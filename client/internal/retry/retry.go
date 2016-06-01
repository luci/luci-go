// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package retry

import (
	"io"
	"log"
	"time"
)

// Default defines the default retry parameters that should be used throughout
// the program. It is fine to update this variable on start up.
var Default = &Config{
	10,
	100 * time.Millisecond,
	500 * time.Millisecond,
	5 * time.Second,
}

// Config defines the retry properties.
type Config struct {
	MaxTries            int           // Maximum number of retries.
	SleepMax            time.Duration // Maximum duration of a single sleep.
	SleepBase           time.Duration // Base sleep duration.
	SleepMultiplicative time.Duration // Incremental sleep duration for each additional try.
}

// Do runs a Retriable, potentially retrying it multiple times.
func (c *Config) Do(r Retriable) (err error) {
	defer func() {
		if err2 := r.Close(); err == nil {
			err = err2
		}
	}()
	for i := 0; i < c.MaxTries; i++ {
		err = r.Do()
		if _, ok := err.(Error); !ok {
			return err
		}
		if i != c.MaxTries-1 {
			s := c.SleepBase + time.Duration(i)*c.SleepMultiplicative
			if s > c.SleepMax {
				s = c.SleepMax
			}
			if s != 0 {
				time.Sleep(s)
			}
			log.Printf("Task failed, retrying after a sleep of %s: %s", s, err)
		}
	}
	return
}

// Error is an error that can be retried.
type Error struct {
	Err error
}

func (e Error) Error() string {
	return e.Err.Error()
}

// Retriable is a task that can be retried. It is important that Do be
// idempotent.
type Retriable interface {
	io.Closer
	Do() error
}
