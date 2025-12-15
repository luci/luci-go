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

package aip160

import "testing"

func TestTokenKinds(t *testing.T) {
	tests := []struct {
		input string
		kind  string
		value string
	}{
		{input: "<= 10", kind: kindComparator, value: "<="},
		{input: "-file", kind: kindNegate, value: "-"},
		{input: "NOT file", kind: kindNegate, value: "NOT"},
		{input: "AND b", kind: kindAnd, value: "AND"},
		{input: "OR a", kind: kindOr, value: "OR"},
		{input: ".field", kind: kindDot, value: "."},
		{input: ".\"field\"", kind: kindDot, value: "."},
		{input: "(arg)", kind: kindLParen, value: "("},
		{input: ")", kind: kindRParen, value: ")"},
		{input: ", arg2)", kind: kindComma, value: ","},
		{input: "text", kind: kindText, value: "text"},
		{input: "\"string\"", kind: kindString, value: "\"string\""},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			token, err := NewLexer(test.input).Next()
			if err != nil {
				t.Fatalf("Error when lexing: %v", err)
			}
			if token.kind != test.kind {
				t.Errorf("Wrong kind: got %s, want %s", token.kind, test.kind)
			}
			if token.value != test.value {
				t.Errorf("Wrong kind: got %s, want %s", token.value, test.value)
			}
		})
	}
}

func TestWhitespaceLexing(t *testing.T) {
	filter := "text \"string with whitespace\" (43 AND 44) OR 45 NOT function(arg1, arg2):hello -field1.field2: hello field < 36"
	tokens := []token{
		{kind: kindText, value: "text"},
		{kind: kindString, value: "\"string with whitespace\""},
		{kind: kindLParen, value: "("},
		{kind: kindText, value: "43"},
		{kind: kindAnd, value: "AND"},
		{kind: kindText, value: "44"},
		{kind: kindRParen, value: ")"},
		{kind: kindOr, value: "OR"},
		{kind: kindText, value: "45"},
		{kind: kindNegate, value: "NOT"},
		{kind: kindText, value: "function"},
		{kind: kindLParen, value: "("},
		{kind: kindText, value: "arg1"},
		{kind: kindComma, value: ","},
		{kind: kindText, value: "arg2"},
		{kind: kindRParen, value: ")"},
		{kind: kindComparator, value: ":"},
		{kind: kindText, value: "hello"},
		{kind: kindNegate, value: "-"},
		{kind: kindText, value: "field1"},
		{kind: kindDot, value: "."},
		{kind: kindText, value: "field2"},
		{kind: kindComparator, value: ":"},
		{kind: kindText, value: "hello"},
		{kind: kindText, value: "field"},
		{kind: kindComparator, value: "<"},
		{kind: kindText, value: "36"},
		{kind: kindEnd, value: ""},
		{kind: kindEnd, value: ""},
	}
	l := NewLexer(filter)
	for i, expected := range tokens {
		actual, err := l.Next()
		if err != nil {
			t.Fatalf("Error getting next token: %v", err)
		}
		if actual.kind != expected.kind {
			t.Errorf("wrong token kind for token %d: got %s, want %s", i, actual.kind, expected.kind)
		}
		if actual.value != expected.value {
			t.Errorf("wrong token value for token %d: got %s, want %s", i, actual.value, expected.value)
		}
	}
}

func TestFullParse(t *testing.T) {
	tests := []struct {
		input     string
		ast       string
		expectErr bool
	}{
		{input: "", ast: "filter{}"},
		{input: " ", ast: "filter{}"},
		{input: "simple", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"simple\"}}}}}}}}}}"},
		{input: " wsBefore", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"wsBefore\"}}}}}}}}}}"},
		{input: "wsAfter ", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"wsAfter\"}}}}}}}}}}"},
		{input: " wsAround ", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"wsAround\"}}}}}}}}}}"},
		{input: "\"string\"", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{quoted,\"string\"}}}}}}}}}}"},
		{input: " \"string\" ", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{quoted,\"string\"}}}}}}}}}}"},
		{input: "\"ws string\"", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{quoted,\"ws string\"}}}}}}}}}}"},
		{input: "-negated", ast: "filter{expression{sequence{factor{term{-simple{restriction{comparable{member{value{\"negated\"}}}}}}}}}}"},
		{input: " - negated ", ast: "filter{expression{sequence{factor{term{-simple{restriction{comparable{member{value{\"negated\"}}}}}}}}}}"},
		// This is a common case (lots of test names are separated by -).
		{input: "dash-separated-name", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"dash-separated-name\"}}}}}}}}}}"},
		{input: "term -negated-term", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"term\"}}}}}}},factor{term{-simple{restriction{comparable{member{value{\"negated-term\"}}}}}}}}}}"},
		{input: "NOT negated", ast: "filter{expression{sequence{factor{term{-simple{restriction{comparable{member{value{\"negated\"}}}}}}}}}}"},
		{input: " NOT negated ", ast: "filter{expression{sequence{factor{term{-simple{restriction{comparable{member{value{\"negated\"}}}}}}}}}}"},
		{input: " NOTnegated ", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"NOTnegated\"}}}}}}}}}}"},
		{input: "implicit and", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"implicit\"}}}}}}},factor{term{simple{restriction{comparable{member{value{\"and\"}}}}}}}}}}"},
		{input: " implicit and ", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"implicit\"}}}}}}},factor{term{simple{restriction{comparable{member{value{\"and\"}}}}}}}}}}"},
		{input: "explicit AND and", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"explicit\"}}}}}}}},sequence{factor{term{simple{restriction{comparable{member{value{\"and\"}}}}}}}}}}"},
		{input: "explicit AND ", expectErr: true},
		{input: "explicit AND and", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"explicit\"}}}}}}}},sequence{factor{term{simple{restriction{comparable{member{value{\"and\"}}}}}}}}}}"},
		{input: " explicit AND and ", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"explicit\"}}}}}}}},sequence{factor{term{simple{restriction{comparable{member{value{\"and\"}}}}}}}}}}"},
		{input: " explicit ANDnotand ", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"explicit\"}}}}}}},factor{term{simple{restriction{comparable{member{value{\"ANDnotand\"}}}}}}}}}}"},
		{input: "test OR or", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"test\"}}}}}},term{simple{restriction{comparable{member{value{\"or\"}}}}}}}}}}"},
		{input: "test OR ", expectErr: true},
		{input: "test ORnotor", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"test\"}}}}}}},factor{term{simple{restriction{comparable{member{value{\"ORnotor\"}}}}}}}}}}"},
		{input: " test OR or ", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"test\"}}}}}},term{simple{restriction{comparable{member{value{\"or\"}}}}}}}}}}"},
		{input: " testORor ", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"testORor\"}}}}}}}}}}"},
		{input: "implicit and AND explicit", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"implicit\"}}}}}}},factor{term{simple{restriction{comparable{member{value{\"and\"}}}}}}}},sequence{factor{term{simple{restriction{comparable{member{value{\"explicit\"}}}}}}}}}}"},
		{input: "implicit with OR term AND explicit OR term", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"implicit\"}}}}}}},factor{term{simple{restriction{comparable{member{value{\"with\"}}}}}},term{simple{restriction{comparable{member{value{\"term\"}}}}}}}},sequence{factor{term{simple{restriction{comparable{member{value{\"explicit\"}}}}}},term{simple{restriction{comparable{member{value{\"term\"}}}}}}}}}}"},
		{input: "(composite)", ast: "filter{expression{sequence{factor{term{simple{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"composite\"}}}}}}}}}}}}}}}"},
		{input: " (composite) ", ast: "filter{expression{sequence{factor{term{simple{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"composite\"}}}}}}}}}}}}}}}"},
		{input: "( composite )", ast: "filter{expression{sequence{factor{term{simple{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"composite\"}}}}}}}}}}}}}}}"},
		{input: " ( composite ) ", ast: "filter{expression{sequence{factor{term{simple{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"composite\"}}}}}}}}}}}}}}}"},
		{input: " ( composite multi) ", ast: "filter{expression{sequence{factor{term{simple{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"composite\"}}}}}}},factor{term{simple{restriction{comparable{member{value{\"multi\"}}}}}}}}}}}}}}}"},
		{input: "value<21", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"value\"}}},\"<\",arg{comparable{member{value{\"21\"}}}}}}}}}}}"},
		{input: "value < 21", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"value\"}}},\"<\",arg{comparable{member{value{\"21\"}}}}}}}}}}}"},
		{input: " value < 21 ", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"value\"}}},\"<\",arg{comparable{member{value{\"21\"}}}}}}}}}}}"},
		{input: "value<=21", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"value\"}}},\"<=\",arg{comparable{member{value{\"21\"}}}}}}}}}}}"},
		{input: "value>21", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"value\"}}},\">\",arg{comparable{member{value{\"21\"}}}}}}}}}}}"},
		{input: "value>=21", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"value\"}}},\">=\",arg{comparable{member{value{\"21\"}}}}}}}}}}}"},
		{input: "value=21", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"value\"}}},\"=\",arg{comparable{member{value{\"21\"}}}}}}}}}}}"},
		{input: "value!=21", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"value\"}}},\"!=\",arg{comparable{member{value{\"21\"}}}}}}}}}}}"},
		{input: "value:21", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"value\"}}},\":\",arg{comparable{member{value{\"21\"}}}}}}}}}}}"},
		{input: "value=(composite)", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"value\"}}},\"=\",arg{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"composite\"}}}}}}}}}}}}}}}}}"},
		// Note: although this parses correctly as a "global" restriction, the implementation doesn't handle this type of restriction, so an error will be returned higher in the stack.
		{input: "member.field", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"member\"}, {value{\"field\"}}}}}}}}}}}"},
		{input: " member.field > 4 ", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"member\"}, {value{\"field\"}}}},\">\",arg{comparable{member{value{\"4\"}}}}}}}}}}}"},
		{input: " member.\"field\" > 4 ", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"member\"}, {value{quoted,\"field\"}}}},\">\",arg{comparable{member{value{\"4\"}}}}}}}}}}}"},
		{input: "composite (expression)", ast: "filter{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"composite\"}}}}}}},factor{term{simple{expression{sequence{factor{term{simple{restriction{comparable{member{value{\"expression\"}}}}}}}}}}}}}}}"},
		// This should parse as a function, but function parsing is not implemented.
		// {input: "function(expression)", ast: ""},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			filter, err := ParseFilter(test.input)
			if test.expectErr {
				if err == nil {
					t.Fatalf("expected error but no error produced from input: %q\nparsed as:%q", test.input, filter.String())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			ast := filter.String()
			if ast != test.ast {
				t.Errorf("incorrect AST parsed from input %q:\ngot %q\nwant %q", test.input, ast, test.ast)
			}
		})
	}
}
