package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"text/template"
	"time"

	"github.com/golang/protobuf/jsonpb"

	"go.chromium.org/luci/buildbucket/cmd/bbagent/bbinput"
)

// the disappointed emoji `{{- "" -}}` removes whitespace before "usage:"
var helpTemplate = template.Must(template.New("").Parse(`{{- "" -}}
usage:
	{{.Prog}} <<<"raw argument to bbagent"
	{{.Prog}} "raw argument to bbagent"
	{{.Prog}} help

Decodes and prints the BBAgentArgs message from the raw input to bbagent.

Useful for debugging swarming tasks which use bbagent in order to see what
buildbucket passed as the input message.
`))

func help() {
	helpTemplate.Execute(os.Stdout, map[string]string{
		"Prog": os.Args[0],
	})
	os.Exit(0)
}

func main() {
	var raw string

	interrupt := make(chan os.Signal)
	go func() {
		<-interrupt
		help()
	}()
	signal.Notify(interrupt, os.Interrupt)

	switch {
	case len(os.Args) == 1:
		done := make(chan struct{})
		var data []byte
		go func() {
			defer close(done)
			var err error
			data, err = ioutil.ReadAll(os.Stdin)
			if err != nil {
				panic(err)
			}
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			fmt.Fprintf(os.Stderr, "waiting for bbagent string on stdin...\n")
			<-done
		}
		raw = string(data)
		if len(raw) == 0 {
			help()
		}

	case len(os.Args) == 2:
		arg := os.Args[1]
		switch arg {
		case "-h", "--help", "help":
			help()
		default:
			raw = arg
		}

	default:
		help()
	}

	bbargs, err := bbinput.Parse(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse: %s", err)
		os.Exit(1)
	}
	err = (&jsonpb.Marshaler{OrigName: true, Indent: ""}).Marshal(os.Stdout, bbargs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to emit: %s", err)
		os.Exit(1)
	}
}
