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

	"go.chromium.org/luci/analysis/internal/clustering"
	analysispb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRules(t *testing.T) {
	Convey(`Syntax Parsing`, t, func() {
		parse := func(input string) error {
			expr, err := Parse(input)
			if err != nil {
				So(expr, ShouldBeNil)
			} else {
				So(expr, ShouldNotBeNil)
			}
			return err
		}
		Convey(`Valid inputs`, func() {
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
				So(parse(v), ShouldBeNil)
			}
		})
		Convey(`Invalid inputs`, func() {
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
				So(parse(v), ShouldNotBeNil)
			}
		})
	})
	Convey(`Semantics`, t, func() {
		eval := func(input string, failure *clustering.Failure) bool {
			eval, err := Parse(input)
			So(err, ShouldBeNil)
			return eval.eval(failure)
		}
		boot := &clustering.Failure{
			TestID: "tast.arc.Boot",
			Reason: &analysispb.FailureReason{PrimaryErrorMessage: "annotation 1: annotation 2: failure"},
		}
		dbus := &clustering.Failure{
			TestID: "tast.example.DBus",
			Reason: &analysispb.FailureReason{PrimaryErrorMessage: "true was not true"},
		}
		Convey(`String Expression`, func() {
			So(eval(`test = "tast.arc.Boot"`, boot), ShouldBeTrue)
			So(eval(`test = "tast.arc.Boot"`, dbus), ShouldBeFalse)
			So(eval(`test = test`, dbus), ShouldBeTrue)
			escaping := &clustering.Failure{
				TestID: "\a\b\f\n\r\t\v\"\101\x42\u0042\U00000042",
			}
			So(eval(`test = "\a\b\f\n\r\t\v\"\101\x42\u0042\U00000042"`, escaping), ShouldBeTrue)
		})
		Convey(`Boolean Constants`, func() {
			So(eval(`TRUE`, boot), ShouldBeTrue)
			So(eval(`tRue`, boot), ShouldBeTrue)
			So(eval(`FALSE`, boot), ShouldBeFalse)
		})
		Convey(`Boolean Item`, func() {
			So(eval(`(((TRUE)))`, boot), ShouldBeTrue)
			So(eval(`(FALSE)`, boot), ShouldBeFalse)
		})
		Convey(`Boolean Predicate`, func() {
			Convey(`Comp`, func() {
				So(eval(`test = "tast.arc.Boot"`, boot), ShouldBeTrue)
				So(eval(`test = "tast.arc.Boot"`, dbus), ShouldBeFalse)
				So(eval(`test <> "tast.arc.Boot"`, boot), ShouldBeFalse)
				So(eval(`test <> "tast.arc.Boot"`, dbus), ShouldBeTrue)
				So(eval(`test != "tast.arc.Boot"`, boot), ShouldBeFalse)
				So(eval(`test != "tast.arc.Boot"`, dbus), ShouldBeTrue)
			})
			Convey(`Negatable`, func() {
				So(eval(`test NOT LIKE "tast.arc.%"`, boot), ShouldBeFalse)
				So(eval(`test NOT LIKE "tast.arc.%"`, dbus), ShouldBeTrue)
				So(eval(`test LIKE "tast.arc.%"`, boot), ShouldBeTrue)
				So(eval(`test LIKE "tast.arc.%"`, dbus), ShouldBeFalse)
			})
			Convey(`Like`, func() {
				So(eval(`test LIKE "tast.arc.%"`, boot), ShouldBeTrue)
				So(eval(`test LIKE "tast.arc.%"`, dbus), ShouldBeFalse)
				So(eval(`test LIKE "arc.%"`, boot), ShouldBeFalse)
				So(eval(`test LIKE ".Boot"`, boot), ShouldBeFalse)
				So(eval(`test LIKE "%arc.%"`, boot), ShouldBeTrue)
				So(eval(`test LIKE "%.Boot"`, boot), ShouldBeTrue)
				So(eval(`test LIKE "tast.%.Boot"`, boot), ShouldBeTrue)

				escapeTest := &clustering.Failure{
					TestID: "a\\.+*?()|[]{}^$a",
				}
				So(eval(`test LIKE "\\\\.+*?()|[]{}^$a"`, escapeTest), ShouldBeFalse)
				So(eval(`test LIKE "a\\\\.+*?()|[]{}^$"`, escapeTest), ShouldBeFalse)
				So(eval(`test LIKE "a\\\\.+*?()|[]{}^$a"`, escapeTest), ShouldBeTrue)
				So(eval(`test LIKE "a\\\\.+*?()|[]{}^$_"`, escapeTest), ShouldBeTrue)
				So(eval(`test LIKE "a\\\\.+*?()|[]{}^$%"`, escapeTest), ShouldBeTrue)

				escapeTest2 := &clustering.Failure{
					TestID: "a\\.+*?()|[]{}^$_",
				}
				So(eval(`test LIKE "a\\\\.+*?()|[]{}^$\\_"`, escapeTest), ShouldBeFalse)
				So(eval(`test LIKE "a\\\\.+*?()|[]{}^$\\_"`, escapeTest2), ShouldBeTrue)

				escapeTest3 := &clustering.Failure{
					TestID: "a\\.+*?()|[]{}^$%",
				}

				So(eval(`test LIKE "a\\\\.+*?()|[]{}^$\\%"`, escapeTest), ShouldBeFalse)
				So(eval(`test LIKE "a\\\\.+*?()|[]{}^$\\%"`, escapeTest3), ShouldBeTrue)

				escapeTest4 := &clustering.Failure{
					Reason: &analysispb.FailureReason{
						PrimaryErrorMessage: "a\nb",
					},
				}
				So(eval(`reason LIKE "a"`, escapeTest4), ShouldBeFalse)
				So(eval(`reason LIKE "%"`, escapeTest4), ShouldBeTrue)
				So(eval(`reason LIKE "a%b"`, escapeTest4), ShouldBeTrue)
				So(eval(`reason LIKE "a_b"`, escapeTest4), ShouldBeTrue)
			})
			Convey(`In`, func() {
				So(eval(`test IN ("tast.arc.Boot")`, boot), ShouldBeTrue)
				So(eval(`test IN ("tast.arc.Clipboard", "tast.arc.Boot")`, boot), ShouldBeTrue)
				So(eval(`test IN ("tast.arc.Clipboard", "tast.arc.Boot")`, dbus), ShouldBeFalse)
			})
		})
		Convey(`Boolean Function`, func() {
			So(eval(`REGEXP_CONTAINS(test, "tast\\.arc\\..*")`, boot), ShouldBeTrue)
			So(eval(`REGEXP_CONTAINS(test, "tast\\.arc\\..*")`, dbus), ShouldBeFalse)
		})
		Convey(`Boolean Factor`, func() {
			So(eval(`NOT TRUE`, boot), ShouldBeFalse)
			So(eval(`NOT FALSE`, boot), ShouldBeTrue)
			So(eval(`NOT REGEXP_CONTAINS(test, "tast\\.arc\\..*")`, boot), ShouldBeFalse)
			So(eval(`NOT REGEXP_CONTAINS(test, "tast\\.arc\\..*")`, dbus), ShouldBeTrue)
		})
		Convey(`Boolean Term`, func() {
			So(eval(`TRUE AND TRUE`, boot), ShouldBeTrue)
			So(eval(`TRUE AND FALSE`, boot), ShouldBeFalse)
			So(eval(`NOT FALSE AND NOT FALSE`, boot), ShouldBeTrue)
			So(eval(`NOT FALSE AND NOT FALSE AND NOT FALSE`, boot), ShouldBeTrue)
		})
		Convey(`Boolean Expression`, func() {
			So(eval(`TRUE OR FALSE`, boot), ShouldBeTrue)
			So(eval(`FALSE AND FALSE OR TRUE`, boot), ShouldBeTrue)
			So(eval(`FALSE AND TRUE OR FALSE OR FALSE AND TRUE`, boot), ShouldBeFalse)
		})
	})
	Convey(`Formatting`, t, func() {
		roundtrip := func(input string) string {
			eval, err := Parse(input)
			So(err, ShouldBeNil)
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
			So(roundtrip(input), ShouldEqual, input)
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
	data := &clustering.Failure{
		TestID: testText.String(),
		Reason: &analysispb.FailureReason{PrimaryErrorMessage: reasonText.String()},
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
