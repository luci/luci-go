// Copyright 2022 The LUCI Authors.
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

package lang

import (
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRules(t *testing.T) {
	ftt.Run(`Syntax Parsing`, t, func(t *ftt.Test) {
		parse := func(input string) error {
			expr, err := Parse(input)
			if err != nil {
				assert.Loosely(t, expr, should.BeNil)
			} else {
				assert.Loosely(t, expr, should.NotBeNil)
			}
			return err
		}
		t.Run(`Valid inputs`, func(t *ftt.Test) {
			validInputs := []string{
				`false`,
				`true`,
				`true or true and not true`,
				`(((true)))`,
				`"" = "foo"`,
				`"" = "'"`,
				`"" = "\a\b\f\n\r\t\v\"\101\x42\u0042\U00000042\ufffd"`,
				`"" = test`,
				`"" = TesT`,
				`test = "foo"`,
				`test != "foo"`,
				`test <> "foo"`,
				`test in ("foo", "bar", reason)`,
				`test not in ("foo", "bar", reason)`,
				`not test in ("foo", "bar", reason)`,
				`test like "%arc%"`,
				`test not like "%arc%"`,
				`not test like "%arc%"`,
				`regexp_contains (test, "^arc\\.")`,
				`not regexp_contains(test, "^arc\\.")`,
				`test = "arc.Boot" AND reason LIKE "%failed%"`,
			}
			for _, v := range validInputs {
				assert.Loosely(t, parse(v), should.BeNil)
			}
		})
		t.Run(`Invalid inputs`, func(t *ftt.Test) {
			invalidInputs := []string{
				`'' = 'foo'`,                  // Uses single quotes.
				`"" = "\xff"`,                 // Illegal Unicode string.
				`"" = "\xff\xfb"`,             // Illegal Unicode string.
				`"" = "\ud800"`,               // Illegal Unicode surrogate code point (D800-DFFF).
				`"" = "\udfff"`,               // Illegal Unicode surrogate code point (D800-DFFF).
				`"" = "\U00110000"`,           // Above maximum Unicode code point (10FFFF).
				`"" = "\c"`,                   // Illegal escape sequence.
				`"" = foo`,                    // Bad identifier.
				`"" = ?`,                      // Bad identifier.
				`test $ "foo"`,                // Invalid operator.
				`test like build`,             // Use of non-constant like pattern.
				`regexp_contains(test, "[")`,  // bad regexp.
				`reason like "foo\\"`,         // invalid trailing "\" escape sequence in LIKE pattern.
				`reason like "foo\\a"`,        // invalid escape sequence "\a" in LIKE pattern.
				`regexp_contains(test, test)`, // Use of non-constant regexp pattern.
				`regexp_contains(test)`,       // Incorrect argument count.
				`bad_func(test, test)`,        // Undeclared function.
				`reason NOTLIKE "%failed%"`,   // Bad operator.
			}
			for _, v := range invalidInputs {
				assert.Loosely(t, parse(v), should.NotBeNil)
			}
		})
	})
	ftt.Run(`Semantics`, t, func(t *ftt.Test) {
		eval := func(input string, failure Failure) bool {
			eval, err := Parse(input)
			assert.Loosely(t, err, should.BeNil)
			return eval.eval(failure)
		}
		boot := Failure{
			Test:   "tast.arc.Boot",
			Reason: "annotation 1: annotation 2: failure",
		}
		dbus := Failure{
			Test:   "tast.example.DBus",
			Reason: "true was not true",
		}
		t.Run(`String Expression`, func(t *ftt.Test) {
			assert.Loosely(t, eval(`test = "tast.arc.Boot"`, boot), should.BeTrue)
			assert.Loosely(t, eval(`test = "tast.arc.Boot"`, dbus), should.BeFalse)
			assert.Loosely(t, eval(`test = test`, dbus), should.BeTrue)
			escaping := Failure{
				Test: "\a\b\f\n\r\t\v\"\101\x42\u0042\U00000042",
			}
			assert.Loosely(t, eval(`test = "\a\b\f\n\r\t\v\"\101\x42\u0042\U00000042"`, escaping), should.BeTrue)
		})
		t.Run(`Boolean Constants`, func(t *ftt.Test) {
			assert.Loosely(t, eval(`TRUE`, boot), should.BeTrue)
			assert.Loosely(t, eval(`tRue`, boot), should.BeTrue)
			assert.Loosely(t, eval(`FALSE`, boot), should.BeFalse)
		})
		t.Run(`Boolean Item`, func(t *ftt.Test) {
			assert.Loosely(t, eval(`(((TRUE)))`, boot), should.BeTrue)
			assert.Loosely(t, eval(`(FALSE)`, boot), should.BeFalse)
		})
		t.Run(`Boolean Predicate`, func(t *ftt.Test) {
			t.Run(`Comp`, func(t *ftt.Test) {
				assert.Loosely(t, eval(`test = "tast.arc.Boot"`, boot), should.BeTrue)
				assert.Loosely(t, eval(`test = "tast.arc.Boot"`, dbus), should.BeFalse)
				assert.Loosely(t, eval(`test <> "tast.arc.Boot"`, boot), should.BeFalse)
				assert.Loosely(t, eval(`test <> "tast.arc.Boot"`, dbus), should.BeTrue)
				assert.Loosely(t, eval(`test != "tast.arc.Boot"`, boot), should.BeFalse)
				assert.Loosely(t, eval(`test != "tast.arc.Boot"`, dbus), should.BeTrue)
			})
			t.Run(`Negatable`, func(t *ftt.Test) {
				assert.Loosely(t, eval(`test NOT LIKE "tast.arc.%"`, boot), should.BeFalse)
				assert.Loosely(t, eval(`test NOT LIKE "tast.arc.%"`, dbus), should.BeTrue)
				assert.Loosely(t, eval(`test LIKE "tast.arc.%"`, boot), should.BeTrue)
				assert.Loosely(t, eval(`test LIKE "tast.arc.%"`, dbus), should.BeFalse)
			})
			t.Run(`Like`, func(t *ftt.Test) {
				assert.Loosely(t, eval(`test LIKE "tast.arc.%"`, boot), should.BeTrue)
				assert.Loosely(t, eval(`test LIKE "tast.arc.%"`, dbus), should.BeFalse)
				assert.Loosely(t, eval(`test LIKE "arc.%"`, boot), should.BeFalse)
				assert.Loosely(t, eval(`test LIKE ".Boot"`, boot), should.BeFalse)
				assert.Loosely(t, eval(`test LIKE "%arc.%"`, boot), should.BeTrue)
				assert.Loosely(t, eval(`test LIKE "%.Boot"`, boot), should.BeTrue)
				assert.Loosely(t, eval(`test LIKE "tast.%.Boot"`, boot), should.BeTrue)

				escapeTest := Failure{
					Test: "a\\.+*?()|[]{}^$a",
				}
				assert.Loosely(t, eval(`test LIKE "\\\\.+*?()|[]{}^$a"`, escapeTest), should.BeFalse)
				assert.Loosely(t, eval(`test LIKE "a\\\\.+*?()|[]{}^$"`, escapeTest), should.BeFalse)
				assert.Loosely(t, eval(`test LIKE "a\\\\.+*?()|[]{}^$a"`, escapeTest), should.BeTrue)
				assert.Loosely(t, eval(`test LIKE "a\\\\.+*?()|[]{}^$_"`, escapeTest), should.BeTrue)
				assert.Loosely(t, eval(`test LIKE "a\\\\.+*?()|[]{}^$%"`, escapeTest), should.BeTrue)

				escapeTest2 := Failure{
					Test: "a\\.+*?()|[]{}^$_",
				}
				assert.Loosely(t, eval(`test LIKE "a\\\\.+*?()|[]{}^$\\_"`, escapeTest), should.BeFalse)
				assert.Loosely(t, eval(`test LIKE "a\\\\.+*?()|[]{}^$\\_"`, escapeTest2), should.BeTrue)

				escapeTest3 := Failure{
					Test: "a\\.+*?()|[]{}^$%",
				}

				assert.Loosely(t, eval(`test LIKE "a\\\\.+*?()|[]{}^$\\%"`, escapeTest), should.BeFalse)
				assert.Loosely(t, eval(`test LIKE "a\\\\.+*?()|[]{}^$\\%"`, escapeTest3), should.BeTrue)

				escapeTest4 := Failure{
					Reason: "a\nb",
				}
				assert.Loosely(t, eval(`reason LIKE "a"`, escapeTest4), should.BeFalse)
				assert.Loosely(t, eval(`reason LIKE "%"`, escapeTest4), should.BeTrue)
				assert.Loosely(t, eval(`reason LIKE "a%b"`, escapeTest4), should.BeTrue)
				assert.Loosely(t, eval(`reason LIKE "a_b"`, escapeTest4), should.BeTrue)
			})
			t.Run(`In`, func(t *ftt.Test) {
				assert.Loosely(t, eval(`test IN ("tast.arc.Boot")`, boot), should.BeTrue)
				assert.Loosely(t, eval(`test IN ("tast.arc.Clipboard", "tast.arc.Boot")`, boot), should.BeTrue)
				assert.Loosely(t, eval(`test IN ("tast.arc.Clipboard", "tast.arc.Boot")`, dbus), should.BeFalse)
			})
		})
		t.Run(`Boolean Function`, func(t *ftt.Test) {
			assert.Loosely(t, eval(`REGEXP_CONTAINS(test, "tast\\.arc\\..*")`, boot), should.BeTrue)
			assert.Loosely(t, eval(`REGEXP_CONTAINS(test, "tast\\.arc\\..*")`, dbus), should.BeFalse)
		})
		t.Run(`Boolean Factor`, func(t *ftt.Test) {
			assert.Loosely(t, eval(`NOT TRUE`, boot), should.BeFalse)
			assert.Loosely(t, eval(`NOT FALSE`, boot), should.BeTrue)
			assert.Loosely(t, eval(`NOT REGEXP_CONTAINS(test, "tast\\.arc\\..*")`, boot), should.BeFalse)
			assert.Loosely(t, eval(`NOT REGEXP_CONTAINS(test, "tast\\.arc\\..*")`, dbus), should.BeTrue)
		})
		t.Run(`Boolean Term`, func(t *ftt.Test) {
			assert.Loosely(t, eval(`TRUE AND TRUE`, boot), should.BeTrue)
			assert.Loosely(t, eval(`TRUE AND FALSE`, boot), should.BeFalse)
			assert.Loosely(t, eval(`NOT FALSE AND NOT FALSE`, boot), should.BeTrue)
			assert.Loosely(t, eval(`NOT FALSE AND NOT FALSE AND NOT FALSE`, boot), should.BeTrue)
		})
		t.Run(`Boolean Expression`, func(t *ftt.Test) {
			assert.Loosely(t, eval(`TRUE OR FALSE`, boot), should.BeTrue)
			assert.Loosely(t, eval(`FALSE AND FALSE OR TRUE`, boot), should.BeTrue)
			assert.Loosely(t, eval(`FALSE AND TRUE OR FALSE OR FALSE AND TRUE`, boot), should.BeFalse)
		})
	})
	ftt.Run(`Formatting`, t, func(t *ftt.Test) {
		roundtrip := func(input string) string {
			eval, err := Parse(input)
			assert.Loosely(t, err, should.BeNil)
			return eval.String()
		}
		// The following statements should be formatted exactly the same when they are printed.
		inputs := []string{
			`FALSE`,
			`TRUE`,
			`TRUE OR TRUE AND NOT TRUE`,
			`(((TRUE)))`,
			`"" = "foo"`,
			`"" = "'"`,
			`"" = "\a\b\f\n\r\t\v\"\101\x42\u0042\U00000042"`,
			`"" = test`,
			`test = "foo"`,
			`test != "foo"`,
			`test <> "foo"`,
			`test IN ("foo", "bar", reason)`,
			`test NOT IN ("foo", "bar", reason)`,
			`NOT test IN ("foo", "bar", reason)`,
			`test LIKE "%arc%"`,
			`test NOT LIKE "%arc%"`,
			`NOT test LIKE "%arc%"`,
			`regexp_contains(test, "^arc\\.")`,
			`NOT regexp_contains(test, "^arc\\.")`,
			`test = "arc.Boot" AND reason LIKE "%failed%"`,
		}
		for _, input := range inputs {
			assert.Loosely(t, roundtrip(input), should.Equal(input))
		}
	})
}

// On my machine, I get the following reuslts:
// cpu: Intel(R) Xeon(R) CPU @ 2.00GHz
// BenchmarkRules-48    	      51	  22406568 ns/op	     481 B/op	       0 allocs/op
func BenchmarkRules(b *testing.B) {
	// Setup 1000 rules.
	var rules []*Expr
	for i := 0; i < 1000; i++ {
		rule := `test LIKE "%arc.Boot` + fmt.Sprintf("%v", i) + `.%" AND reason LIKE "%failed` + fmt.Sprintf("%v", i) + `.%"`
		expr, err := Parse(rule)
		if err != nil {
			b.Error(err)
		}
		rules = append(rules, expr)
	}
	var testText strings.Builder
	var reasonText strings.Builder
	for j := 0; j < 100; j++ {
		testText.WriteString("blah")
		reasonText.WriteString("blah")
	}
	testText.WriteString("arc.Boot0.")
	reasonText.WriteString("failed0.")
	for j := 0; j < 100; j++ {
		testText.WriteString("blah")
		reasonText.WriteString("blah")
	}
	data := Failure{
		Test:   testText.String(),
		Reason: reasonText.String(),
	}

	// Start benchmark.
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for j, r := range rules {
			matches := r.Evaluate(data)
			shouldMatch := j == 0
			if matches != shouldMatch {
				b.Errorf("Unexpected result at %v: got %v, want %v", j, matches, shouldMatch)
			}
		}
	}
}
