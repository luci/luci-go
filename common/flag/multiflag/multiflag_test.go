// Copyright 2016 The LUCI Authors.
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

package multiflag

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type testMultiFlag struct {
	MultiFlag

	S string
	I int
	B bool
}

func (of *testMultiFlag) newOption(name, description string) Option {
	o := &FlagOption{Name: name, Description: description}

	flags := o.Flags()
	flags.StringVar(&of.S, "string-var", "", "A string variable.")
	flags.IntVar(&of.I, "int-var", 123, "An integer variable.")
	flags.BoolVar(&of.B, "bool-var", false, "A boolean variable.")

	// Set our option name.
	return o
}

// A Writer implementation that controllably fails.
type failWriter struct {
	Max   int   // When failing, never "write" more than this many bytes.
	Error error // The error to return when used to write.
}

// Implements io.Writer.
func (w *failWriter) Write(data []byte) (int, error) {
	size := len(data)
	if w.Error != nil && size > w.Max {
		size = w.Max
	}
	return size, w.Error
}

func TestParsing(t *testing.T) {
	ftt.Run("Given empty MultiFlag", t, func(t *ftt.Test) {
		of := &testMultiFlag{}

		t.Run("Its option list should be empty", func(t *ftt.Test) {
			assert.Loosely(t, of.OptionNames(), should.BeEmpty)
		})

		t.Run(`Parsing an option spec with an empty option value should fail.`, func(t *ftt.Test) {
			assert.Loosely(t, of.Parse(`,params`), should.NotBeNil)
		})
	})

	ftt.Run("Given Flags with two option values", t, func(t *ftt.Test) {
		of := &testMultiFlag{}
		of.Options = []Option{
			of.newOption("foo", "Test option 'foo'."),
			of.newOption("bar", "Test option 'bar'."),
		}

		t.Run("Its option list should be: ['foo', 'bar'].", func(t *ftt.Test) {
			assert.Loosely(t, of.OptionNames(), should.Resemble([]string{"foo", "bar"}))
		})

		t.Run(`When parsing 'foo,string-var="hello world"'`, func(t *ftt.Test) {
			err := of.Parse(`foo,string-var="hello world",bool-var=true`)
			assert.Loosely(t, err, should.BeNil)

			t.Run("The option, 'foo', should be selected.", func(t *ftt.Test) {
				assert.Loosely(t, of.Selected, should.NotBeNil)
				assert.Loosely(t, of.Selected.Descriptor().Name, should.Equal("foo"))
			})

			t.Run(`The value of 'string-var' should be, "hello world".`, func(t *ftt.Test) {
				assert.Loosely(t, of.S, should.Equal("hello world"))
			})

			t.Run(`The value of 'int-var' should be default (123)".`, func(t *ftt.Test) {
				assert.Loosely(t, of.I, should.Equal(123))
			})

			t.Run(`The value of 'bool-var' should be "true".`, func(t *ftt.Test) {
				assert.Loosely(t, of.B, should.Equal(true))
			})
		})
	})
}

func TestHelp(t *testing.T) {
	ftt.Run(`A 'MultiFlag' instance`, t, func(t *ftt.Test) {
		of := &MultiFlag{}

		t.Run(`Uses os.Stderr for output.`, func(t *ftt.Test) {
			assert.Loosely(t, of.GetOutput(), should.Equal(os.Stderr))
		})

		t.Run(`Configured with a simple FlagOption with flags`, func(t *ftt.Test) {
			opt := &FlagOption{
				Name:        "foo",
				Description: "An option, 'foo'.",
			}
			opt.Flags().String("bar", "", "An option, 'bar'.")
			of.Options = []Option{opt}

			t.Run(`Should successfully parse "foo" with no flags.`, func(t *ftt.Test) {
				assert.Loosely(t, of.Parse(`foo`), should.BeNil)
			})

			t.Run(`Should successfully parse "foo" with a "bar" flag.`, func(t *ftt.Test) {
				assert.Loosely(t, of.Parse(`foo,bar="Hello!"`), should.BeNil)
			})

			t.Run(`Should fail to parse a non-existent flag.`, func(t *ftt.Test) {
				assert.Loosely(t, of.Parse(`foo,baz`), should.NotBeNil)
			})
		})
	})

	ftt.Run(`Given a testMultiFlag configured with 'nil' output.`, t, func(t *ftt.Test) {
		of := &testMultiFlag{}

		t.Run(`Should default to os.Stderr`, func(t *ftt.Test) {
			assert.Loosely(t, of.GetOutput(), should.Equal(os.Stderr))
		})
	})

	ftt.Run("Given Flags with two options, one of which is 'help'", t, func(t *ftt.Test) {
		var buf bytes.Buffer
		of := &testMultiFlag{}
		of.Output = &buf
		of.Options = []Option{
			HelpOption(&of.MultiFlag),
			of.newOption("foo", "Test option 'foo'."),
		}

		correctHelpString := `
help  Displays this help message. Can be run as "help,<option>" to display help for an option.
foo   Test option 'foo'.
`

		t.Run("Should print a correct help string", func(t *ftt.Test) {
			err := of.PrintHelp()
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, buf.String(), should.Equal(correctHelpString))
		})

		t.Run(`Should fail to print a help string when the writer fails.`, func(t *ftt.Test) {
			w := &failWriter{}
			w.Error = errors.New("fail")
			of.Output = w
			assert.Loosely(t, of.PrintHelp(), should.NotBeNil)
		})

		t.Run(`Should parse a request for a specific option's help string.`, func(t *ftt.Test) {
			err := of.Parse(`help,foo`)
			assert.Loosely(t, err, should.BeNil)

			t.Run(`And should print help for that option.`, func(t *ftt.Test) {
				correctOptionHelpString := `Help for 'foo': Test option 'foo'.
  -bool-var
    	A boolean variable.
  -int-var int
    	An integer variable. (default 123)
  -string-var string
    	A string variable.
`
				assert.Loosely(t, buf.String(), should.Equal(correctOptionHelpString))
			})
		})

		t.Run("Should parse the 'help' option", func(t *ftt.Test) {
			err := of.Parse(`help`)
			assert.Loosely(t, err, should.BeNil)

			t.Run("Should print the correct help string in response.", func(t *ftt.Test) {
				assert.Loosely(t, buf.String(), should.Equal(correctHelpString))
			})
		})

		t.Run("Should parse 'help,junk=1'", func(t *ftt.Test) {
			err := of.Parse(`help,junk=1`)
			assert.Loosely(t, err, should.BeNil)

			t.Run(`Should notify the user that "junk=1" is not an option.`, func(t *ftt.Test) {
				assert.Loosely(t, buf.String(), should.Equal("Unknown option 'junk=1'\n"))
			})
		})
	})
}

func TestHelpItemSlice(t *testing.T) {
	ftt.Run(`Given a slice of testOption instances`, t, func(t *ftt.Test) {
		options := optionDescriptorSlice{
			&OptionDescriptor{
				Name:        "a",
				Description: "An unpinned help item",
				Pinned:      false,
			},
			&OptionDescriptor{
				Name:        "b",
				Description: "Another unpinned help item",
				Pinned:      false,
			},
			&OptionDescriptor{
				Name:        "c",
				Description: "A pinned help item",
				Pinned:      true,
			},
			&OptionDescriptor{
				Name:        "d",
				Description: "Another pinned help item",
				Pinned:      true,
			},
		}
		sort.Sort(options)

		t.Run(`The options should be sorted: c, b, a, b`, func(t *ftt.Test) {
			var names []string
			for _, opt := range options {
				names = append(names, opt.Name)
			}
			assert.Loosely(t, names, should.Resemble([]string{"c", "d", "a", "b"}))
		})
	})
}

func TestFlagParse(t *testing.T) {
	ftt.Run("Given a MultiFlag with one option, 'foo'", t, func(t *ftt.Test) {
		of := &testMultiFlag{}
		of.Options = []Option{
			of.newOption("foo", "Test option 'foo'."),
		}

		t.Run("When configured as an output to a Go flag.MultiFlag", func(t *ftt.Test) {
			gof := &flag.FlagSet{}
			gof.Var(of, "option", "Single line option")

			t.Run(`Should parse '-option foo,string-var="hello world",int-var=1337'`, func(t *ftt.Test) {
				err := gof.Parse([]string{"-option", `foo,string-var="hello world",int-var=1337"`})
				assert.Loosely(t, err, should.BeNil)

				t.Run("Should parse out 'foo' as the option", func(t *ftt.Test) {
					assert.Loosely(t, of.Selected.Descriptor().Name, should.Equal("foo"))
				})

				t.Run(`Should parse out 'string-var' as "hello world".`, func(t *ftt.Test) {
					assert.Loosely(t, of.S, should.Equal("hello world"))
				})

				t.Run(`Should parse out 'int-var' as '1337'.`, func(t *ftt.Test) {
					assert.Loosely(t, of.I, should.Equal(1337))
				})
			})

			t.Run(`When parsing 'bar'`, func(t *ftt.Test) {
				err := gof.Parse([]string{"-option", "bar"})
				assert.Loosely(t, err, should.NotBeNil)
			})
		})
	})
}

func ExampleMultiFlag() {
	// Setup multiflag.
	param := ""
	deprecated := FlagOption{
		Name:        "deprecated",
		Description: "The deprecated option.",
	}
	deprecated.Flags().StringVar(&param, "param", "", "String parameter.")

	beta := FlagOption{Name: "beta", Description: "The new option, which is still beta."}
	beta.Flags().StringVar(&param, "param", "", "Beta string parameter.")

	mf := MultiFlag{
		Description: "My test MultiFlag.",
		Output:      os.Stdout,
	}
	mf.Options = []Option{
		HelpOption(&mf),
		&deprecated,
		&beta,
	}

	// Install the multiflag as a flag in "flags".
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Var(&mf, "multiflag", "Multiflag option.")

	// Parse flags (help).
	cmd := []string{"-multiflag", "help"}
	fmt.Println("Selecting help option:", cmd)
	if err := fs.Parse(cmd); err != nil {
		panic(err)
	}

	// Parse flags (param).
	cmd = []string{"-multiflag", "beta,param=Sup"}
	fmt.Println("Selecting beta option:", cmd)
	if err := fs.Parse(cmd); err != nil {
		panic(err)
	}
	fmt.Printf("Option [%s], parameter: [%s].\n", mf.Selected.Descriptor().Name, param)

	// Output:
	// Selecting help option: [-multiflag help]
	// My test MultiFlag.
	// help        Displays this help message. Can be run as "help,<option>" to display help for an option.
	// beta        The new option, which is still beta.
	// deprecated  The deprecated option.
	// Selecting beta option: [-multiflag beta,param=Sup]
	// Option [beta], parameter: [Sup].
}
