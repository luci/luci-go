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

// This file contains a lexer and parser for AIP-160 filter expressions.
// The EBNF is at https://google.aip.dev/assets/misc/ebnf-filtering.txt
// The function call syntax is not supported which simplifies the parser.
//
// Implemented EBNF (in terms of lexer tokens):
// filter: [expression];
// expression: sequence {WS AND WS sequence};
// sequence: factor {WS factor};
// factor: term {WS OR WS term};
// term: [NEGATE] simple;
// simple: restriction | composite;
// restriction: comparable [COMPARATOR arg];
// comparable: member;
// member: (TEXT | STRING) {DOT (TEXT | STRING)};
// composite: LPAREN expression RPAREN;
// arg: comparable | composite;
//
// TODO(mwarton): Redo whitespace handling.  There are still some cases (like "- 30")
// 				  which are accepted as valid instead of being rejected.
import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	kindComparator = "COMPARATOR"
	kindNegate     = "NEGATE"
	kindAnd        = "AND"
	kindOr         = "OR"
	kindDot        = "DOT"
	kindLParen     = "LPAREN"
	kindRParen     = "RPAREN"
	kindComma      = "COMMA"
	kindString     = "STRING"
	kindText       = "TEXT"
	kindEnd        = "END"
)

// lexerRegexp has one group for each kind of token that can be lexed, in the order of the kind consts above. There are two cases for kindNegate to handle whitespace correctly.
var lexerRegexp = regexp.MustCompile(`^(<=|>=|!=|<|>|=|\:)|(NOT\s)|(-)|(AND\s)|(OR\s)|(\.)|(\()|(\))|(,)|("(?:[^"\\]|\\.)*")|([^\s\.,<>=!:\(\)]+)`)

type token struct {
	kind  string
	value string
}

type filterLexer struct {
	input string
	next  *token
}

func NewLexer(input string) *filterLexer {
	return &filterLexer{input: input}
}

func (l *filterLexer) Peek() (*token, error) {
	if l.next == nil {
		var err error
		l.next, err = l.Next()
		if err != nil {
			return nil, err
		}
	}
	return l.next, nil
}

func (l *filterLexer) Next() (*token, error) {
	if l.next != nil {
		next := l.next
		l.next = nil
		return next, nil
	}
	l.next = nil
	l.input = strings.TrimLeft(l.input, " \t\r\n")
	if l.input == "" {
		return &token{kind: kindEnd}, nil
	}
	matches := lexerRegexp.FindStringSubmatch(l.input)
	if matches == nil {
		return nil, fmt.Errorf("error: unable to lex token from %q", l.input)
	}
	l.input = l.input[len(matches[0]):]
	if matches[1] != "" {
		return &token{kind: kindComparator, value: matches[1]}, nil
	}
	if matches[2] != "" {
		// Needs to be fixed up to compensate for the trailing \s in the match which prevents
		// matching "NOTother" as a negated "other".
		length := len(matches[2])
		return &token{kind: kindNegate, value: matches[2][:length-1]}, nil
	}
	if matches[3] != "" {
		return &token{kind: kindNegate, value: matches[3]}, nil
	}
	if matches[4] != "" {
		// Needs to be fixed up to compensate for the trailing \s in the match which prevents
		// matching "ANDother" as a "AND" "other".
		length := len(matches[4])
		return &token{kind: kindAnd, value: matches[4][:length-1]}, nil
	}
	if matches[5] != "" {
		// Needs to be fixed up to compensate for the trailing \s in the match which prevents
		// matching "ORother" as a "OR" "other".
		length := len(matches[5])
		return &token{kind: kindOr, value: matches[5][:length-1]}, nil
	}
	if matches[6] != "" {
		return &token{kind: kindDot, value: matches[6]}, nil
	}
	if matches[7] != "" {
		return &token{kind: kindLParen, value: matches[7]}, nil
	}
	if matches[8] != "" {
		return &token{kind: kindRParen, value: matches[8]}, nil
	}
	if matches[9] != "" {
		return &token{kind: kindComma, value: matches[9]}, nil
	}
	if matches[10] != "" {
		return &token{kind: kindString, value: matches[10]}, nil
	}
	if matches[11] != "" {
		return &token{kind: kindText, value: matches[11]}, nil
	}
	return nil, fmt.Errorf("error: unhandled lexer regexp match %q", matches[0])
}

// AST Nodes.  These are based on the EBNF at https://google.aip.dev/assets/misc/ebnf-filtering.txt
// Note that the syntax for functions is not currently supported.

// Filter, possibly empty
type Filter struct {
	Expression *Expression // Optional, may be nil.
}

func (v *Filter) String() string {
	var s strings.Builder
	s.WriteString("filter{")
	if v.Expression != nil {
		s.WriteString(v.Expression.String())
	}
	s.WriteString("}")
	return s.String()
}

// Expressions may either be a conjunction (AND) of sequences or a simple
// sequence.
//
// Note, the AND is case-sensitive.
//
// Example: `a b AND c AND d`
//
// The expression `(a b) AND c AND d` is equivalent to the example.
type Expression struct {
	// Sequences are always joined by an AND operator
	Sequences []*Sequence
}

func (v *Expression) String() string {
	var s strings.Builder
	s.WriteString("expression{")
	for i, c := range v.Sequences {
		if i > 0 {
			s.WriteString(",")
		}
		if c != nil {
			s.WriteString(c.String())
		}
	}
	s.WriteString("}")
	return s.String()
}

// Sequence is composed of one or more whitespace (WS) separated factors.
//
// A sequence expresses a logical relationship between 'factors' where
// the ranking of a filter result may be scored according to the number
// factors that match and other such criteria as the proximity of factors
// to each other within a document.
//
// When filters are used with exact match semantics rather than fuzzy
// match semantics, a sequence is equivalent to AND.
//
// Example: `New York Giants OR Yankees`
//
// The expression `New York (Giants OR Yankees)` is equivalent to the
// example.
type Sequence struct {
	// Factors are always joined by an (implicit) AND operator
	Factors []*Factor
}

func (v *Sequence) String() string {
	var s strings.Builder
	s.WriteString("sequence{")
	for i, c := range v.Factors {
		if i > 0 {
			s.WriteString(",")
		}
		if c != nil {
			s.WriteString(c.String())
		}
	}
	s.WriteString("}")
	return s.String()
}

// Factors may either be a disjunction (OR) of terms or a simple term.
//
// Note, the OR is case-sensitive.
//
// Example: `a < 10 OR a >= 100`
type Factor struct {
	// Terms are always joined by an OR operator
	Terms []*Term
}

func (v *Factor) String() string {
	var s strings.Builder
	s.WriteString("factor{")
	for i, c := range v.Terms {
		if i > 0 {
			s.WriteString(",")
		}
		if c != nil {
			s.WriteString(c.String())
		}
	}
	s.WriteString("}")
	return s.String()
}

// Terms may either be unary or simple expressions.
//
// Unary expressions negate the simple expression, either mathematically `-`
// or logically `NOT`. The negation styles may be used interchangeably.
//
// Note, the `NOT` is case-sensitive and must be followed by at least one
// whitespace (WS).
//
// Examples:
// * logical not     : `NOT (a OR b)`
// * alternative not : `-file:".java"`
// * negation        : `-30`
type Term struct {
	Negated bool
	Simple  *Simple
}

func (v *Term) String() string {
	var s strings.Builder
	s.WriteString("term{")
	if v.Negated {
		s.WriteString("-")
	}
	if v.Simple != nil {
		s.WriteString(v.Simple.String())
	}
	s.WriteString("}")
	return s.String()
}

// Simple expressions may either be a restriction or a nested (composite)
// expression.
type Simple struct {
	Restriction *Restriction
	// Composite is a parenthesized expression, commonly used to group
	// terms or clarify operator precedence.
	//
	// Example: `(msg.endsWith('world') AND retries < 10)`
	Composite *Expression
}

func (v *Simple) String() string {
	var s strings.Builder
	s.WriteString("simple{")
	if v.Restriction != nil {
		s.WriteString(v.Restriction.String())
	}
	if v.Restriction != nil && v.Composite != nil {
		s.WriteString(",")
	}
	if v.Composite != nil {
		s.WriteString(v.Composite.String())
	}
	s.WriteString("}")
	return s.String()
}

// Restrictions express a relationship between a comparable value and a
// single argument. When the restriction only specifies a comparable
// without an operator, this is a global restriction.
//
// Note, restrictions are not whitespace sensitive.
//
// Examples:
// * equality         : `package=com.google`
// * inequality       : `msg != 'hello'`
// * greater than     : `1 > 0`
// * greater or equal : `2.5 >= 2.4`
// * less than        : `yesterday < request.time`
// * less or equal    : `experiment.rollout <= cohort(request.user)`
// * has              : `map:key`
// * global           : `prod`
//
// In addition to the global, equality, and ordering operators, filters
// also support the has (`:`) operator. The has operator is unique in
// that it can test for presence or value based on the proto3 type of
// the `comparable` value. The has operator is useful for validating the
// structure and contents of complex values.
type Restriction struct {
	Comparable *Comparable
	// Comparators supported by list filters: <=, <. >=, >, !=, =, :
	Comparator string
	Arg        *Arg
}

func (v *Restriction) String() string {
	var s strings.Builder
	s.WriteString("restriction{")
	if v.Comparable != nil {
		s.WriteString(v.Comparable.String())
	}
	if v.Comparator != "" {
		s.WriteString(",")
		s.WriteString(strconv.Quote(v.Comparator))
	}
	if v.Arg != nil {
		s.WriteString(",")
		s.WriteString(v.Arg.String())
	}
	s.WriteString("}")
	return s.String()
}

type Arg struct {
	Comparable *Comparable
	// Composite is a parenthesized expression, commonly used to group
	// terms or clarify operator precedence.
	//
	// Example: `(msg.endsWith('world') AND retries < 10)`
	Composite *Expression
}

func (v *Arg) String() string {
	var s strings.Builder
	s.WriteString("arg{")
	if v.Comparable != nil {
		s.WriteString(v.Comparable.String())
	}
	if v.Comparable != nil && v.Composite != nil {
		s.WriteString(",")
	}
	if v.Composite != nil {
		s.WriteString(v.Composite.String())
	}
	s.WriteString("}")
	return s.String()
}

// Comparable may either be a member or function.  As functions are not currently supported, it is always a member.
type Comparable struct {
	Member *Member
}

func (v *Comparable) String() string {
	var s strings.Builder
	s.WriteString("comparable{")
	if v.Member != nil {
		s.WriteString(v.Member.String())
	}
	s.WriteString("}")
	return s.String()
}

// Member expressions are either value or DOT qualified field references.
//
// Example: `expr.type_map.1.type`
type Member struct {
	Value  string
	Fields []string
}

func (v *Member) String() string {
	var s strings.Builder
	s.WriteString("member{")
	s.Write([]byte(strconv.Quote(v.Value)))
	if len(v.Fields) > 0 {
		s.WriteString(", {")
	}
	for i, c := range v.Fields {
		if i > 0 {
			s.WriteString(",")
		}
		s.WriteString(strconv.Quote(c))
	}
	s.WriteString("}}")
	return s.String()
}

// Parse an AIP-160 filter string into an AST.
func ParseFilter(filter string) (*Filter, error) {
	return newParser(filter).filter()
}

type parser struct {
	lexer filterLexer
}

func newParser(input string) *parser {
	return &parser{lexer: *NewLexer(input)}
}

func (p *parser) expect(kind string) error {
	t, err := p.lexer.Peek()
	if err != nil {
		return err
	}
	if t.kind != kind {
		return fmt.Errorf("expected %s but got %s(%q)", kind, t.kind, t.value)
	}
	_, err = p.lexer.Next()
	return err
}

func (p *parser) accept(kind string) (*token, error) {
	t, err := p.lexer.Peek()
	if err != nil {
		return nil, err
	}
	if t.kind != kind {
		return nil, nil
	}
	return p.lexer.Next()
}

func (p *parser) filter() (*Filter, error) {
	t, err := p.accept(kindEnd)
	if err != nil {
		return nil, err
	}
	if t != nil {
		return &Filter{}, nil
	}
	e, err := p.expression()
	if err != nil {
		return nil, err
	}
	return &Filter{Expression: e}, p.expect(kindEnd)
}

func (p *parser) expression() (*Expression, error) {
	s, err := p.sequence()
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, nil
	}
	e := &Expression{}
	e.Sequences = append(e.Sequences, s)
	for {
		and, err := p.accept(kindAnd)
		if err != nil {
			return nil, err
		}
		if and == nil {
			break
		}
		s, err := p.sequence()
		if err != nil {
			return nil, err
		}
		if s == nil {
			return nil, fmt.Errorf("expected sequence after AND")
		}
		e.Sequences = append(e.Sequences, s)
	}
	return e, nil
}

func (p *parser) sequence() (*Sequence, error) {
	s := &Sequence{}
	for {
		f, err := p.factor()
		if err != nil {
			return nil, err
		}
		if f == nil {
			break
		}
		s.Factors = append(s.Factors, f)
	}
	if len(s.Factors) == 0 {
		return nil, nil
	}
	return s, nil
}

func (p *parser) factor() (*Factor, error) {
	t, err := p.term()
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	f := &Factor{}
	f.Terms = append(f.Terms, t)
	for {
		or, err := p.accept(kindOr)
		if err != nil {
			return nil, err
		}
		if or == nil {
			break
		}
		t, err := p.term()
		if err != nil {
			return nil, err
		}
		if t == nil {
			return nil, fmt.Errorf("expected sequence after AND")
		}
		f.Terms = append(f.Terms, t)
	}
	return f, nil
}

func (p *parser) term() (*Term, error) {
	n, err := p.accept(kindNegate)
	if err != nil {
		return nil, err
	}
	s, err := p.simple()
	if err != nil {
		return nil, err
	}
	if s == nil {
		if n != nil {
			return nil, fmt.Errorf("expected simple term after negation %q", n.value)
		}
		return nil, nil
	}
	return &Term{Negated: n != nil, Simple: s}, nil
}

func (p *parser) simple() (*Simple, error) {
	r, err := p.restriction()
	if err != nil {
		return nil, err
	}
	if r != nil {
		return &Simple{Restriction: r}, nil
	}
	c, err := p.composite()
	if err != nil {
		return nil, err
	}
	if c != nil {
		return &Simple{Composite: c}, nil
	}
	return nil, nil
}

func (p *parser) restriction() (*Restriction, error) {
	comparable, err := p.comparable()
	if err != nil {
		return nil, err
	}
	if comparable == nil {
		return nil, nil
	}
	comparator, err := p.accept(kindComparator)
	if err != nil {
		return nil, err
	}
	if comparator == nil {
		return &Restriction{Comparable: comparable}, nil
	}
	arg, err := p.arg()
	if err != nil {
		return nil, err
	}
	if arg == nil {
		return nil, fmt.Errorf("expected arg after %s", comparator.value)
	}
	return &Restriction{Comparable: comparable, Comparator: comparator.value, Arg: arg}, nil
}

func (p *parser) comparable() (*Comparable, error) {
	m, err := p.member()
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, nil
	}
	return &Comparable{Member: m}, nil
}

func (p *parser) member() (*Member, error) {
	v, err := p.accept(kindString)
	if err != nil {
		return nil, err
	}
	if v != nil {
		v.value, err = strconv.Unquote(v.value)
		if err != nil {
			return nil, fmt.Errorf("error unquoting string: %w", err)
		}
		return &Member{Value: v.value}, nil
	}

	v, err = p.accept(kindText)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, nil
	}
	m := &Member{Value: v.value}
	for {
		dot, err := p.accept(kindDot)
		if err != nil {
			return nil, err
		}
		if dot == nil {
			break
		}
		f, err := p.accept(kindText)
		if err != nil {
			return nil, err
		}
		if f == nil {
			f, err = p.accept(kindString)
			if err != nil {
				return nil, err
			}
			if f == nil {
				return nil, fmt.Errorf("expected field name after '.'")
			}

			f.value, err = strconv.Unquote(f.value)
			if err != nil {
				return nil, err
			}
		}
		m.Fields = append(m.Fields, f.value)
	}
	return m, nil
}

func (p *parser) composite() (*Expression, error) {
	lparen, err := p.accept(kindLParen)
	if err != nil {
		return nil, err
	}
	if lparen == nil {
		return nil, nil
	}
	e, err := p.expression()
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("expected expression")
	}
	return e, p.expect(kindRParen)
}

func (p *parser) arg() (*Arg, error) {
	comparable, err := p.comparable()
	if err != nil {
		return nil, err
	}
	if comparable != nil {
		return &Arg{Comparable: comparable}, nil
	}
	composite, err := p.composite()
	if err != nil {
		return nil, err
	}
	if composite != nil {
		return &Arg{Composite: composite}, nil
	}
	return nil, nil
}
