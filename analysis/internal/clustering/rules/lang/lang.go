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

// Package lang parses failure association rule predicates. The predicate
// syntax defined here is intended to be a subset of BigQuery Standard SQL's
// Expression syntax, with the same semantics. This provides a few benefits:
//   - Well-known and understood syntax and semantics.
//   - Ability to leverage existing high-quality documentation to communicate
//     language concepts to end-users.
//   - Simplified debugging of LUCI Analysis (by allowing direct copy- paste of
//     expressions into BigQuery to verify clustering is correct).
//   - Possibility of using BigQuery as an execution engine in future.
//
// Rules permitted by this package look similar to:
//
//	reason LIKE "% exited with code 5 %" AND NOT
//	  ( test = "arc.Boot" OR test = "arc.StartStop" )
//
// The grammar for the language in Extended Backus-Naur form follows. The
// top-level production rule is BoolExpr.
//
// BoolExpr = BoolTerm , ( "OR" , BoolTerm )* ;
// BoolTerm = BoolFactor , ( "AND" , BoolFactor )* ;
// BoolFactor = [ "NOT" ] BoolPrimary ;
// BoolPrimary = BoolItem | BoolPredicate ;
// BoolItem = BoolConst | "(" , BoolExpr , ")" | BoolFunc ;
// BoolConst = "TRUE" | "FALSE" ;
// BoolFunc = Identifier , "(" , StringExpr , ( "," , StringExpr )* , ")" ;
// BoolPredicate = StringExpr , BoolTest ;
// BoolTest = CompPredicate | NegatablePredicate ;
// CompPredicate = Operator , StringExpr ;
// Operator = "!=" | "<>" | "="
// NegatablePredicate = [ "NOT" ] , ( InPredicate | LikePredicate ) ;
// InPredicate = "IN" , "(" , StringExpr , ( "," , StringExpr )* , ")" ;
// LikePredicate = "LIKE" , String ;
// StringExpr = String | Identifier ;
//
// Where:
// - Identifier represents the production rule for identifiers.
// - String is the production rule for a double-quoted string literal.
// The precise definitions of which are omitted here but found in the
// implementation.
package lang

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"

	participle "github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/clustering"
)

type validator struct {
	errors []error
}

func newValidator() *validator {
	return &validator{}
}

// ReportError reports a validation error.
func (v *validator) reportError(err error) {
	v.errors = append(v.errors, err)
}

// Error returns all validation errors that were encountered.
func (v *validator) error() error {
	if len(v.errors) > 0 {
		return errors.NewMultiError(v.errors...)
	}
	return nil
}

type failure *clustering.Failure
type boolEval func(failure) bool
type stringEval func(failure) string
type predicateEval func(failure, string) bool

// Expr represents a predicate for a failure association rule.
type Expr struct {
	expr *boolExpr
	eval boolEval
}

// String returns the predicate as a string, with normalised formatting.
func (e *Expr) String() string {
	var buf bytes.Buffer
	e.expr.format(&buf)
	return buf.String()
}

// Evaluate evaluates the given expression, using the given values
// for variables used in the expression.
func (e *Expr) Evaluate(failure *clustering.Failure) bool {
	return e.eval(failure)
}

type boolExpr struct {
	Terms []*boolTerm `parser:"@@ ( 'OR' @@ )*"`
}

func (e *boolExpr) format(w io.Writer) {
	for i, t := range e.Terms {
		if i > 0 {
			io.WriteString(w, " OR ")
		}
		t.format(w)
	}
}

func (e *boolExpr) evaluator(v *validator) boolEval {
	var termEvals []boolEval
	for _, t := range e.Terms {
		termEvals = append(termEvals, t.evaluator(v))
	}
	if len(termEvals) == 1 {
		return termEvals[0]
	}
	return func(f failure) bool {
		for _, termEval := range termEvals {
			if termEval(f) {
				return true
			}
		}
		return false
	}
}

type boolTerm struct {
	Factors []*boolFactor `parser:"@@ ( 'AND' @@ )*"`
}

func (t *boolTerm) format(w io.Writer) {
	for i, f := range t.Factors {
		if i > 0 {
			io.WriteString(w, " AND ")
		}
		f.format(w)
	}
}

func (t *boolTerm) evaluator(v *validator) boolEval {
	var factorEvals []boolEval
	for _, f := range t.Factors {
		factorEvals = append(factorEvals, f.evaluator(v))
	}
	if len(factorEvals) == 1 {
		return factorEvals[0]
	}
	return func(f failure) bool {
		for _, factorEval := range factorEvals {
			if !factorEval(f) {
				return false
			}
		}
		return true
	}
}

type boolFactor struct {
	Not     bool         `parser:"( @'NOT' )?"`
	Primary *boolPrimary `parser:"@@"`
}

func (f *boolFactor) format(w io.Writer) {
	if f.Not {
		io.WriteString(w, "NOT ")
	}
	f.Primary.format(w)
}

func (f *boolFactor) evaluator(v *validator) boolEval {
	predicate := f.Primary.evaluator(v)
	if f.Not {
		return func(f failure) bool {
			return !predicate(f)
		}
	}
	return predicate
}

type boolPrimary struct {
	Item *boolItem      `parser:"@@"`
	Test *boolPredicate `parser:"| @@"`
}

func (p *boolPrimary) format(w io.Writer) {
	if p.Item != nil {
		p.Item.format(w)
	}
	if p.Test != nil {
		p.Test.format(w)
	}
}

func (p *boolPrimary) evaluator(v *validator) boolEval {
	if p.Item != nil {
		return p.Item.evaluator(v)
	}
	return p.Test.evaluator(v)
}

type boolItem struct {
	Const *boolConst    `parser:"@@"`
	Expr  *boolExpr     `parser:"| '(' @@ ')'"`
	Func  *boolFunction `parser:"| @@"`
}

func (i *boolItem) format(w io.Writer) {
	if i.Const != nil {
		i.Const.format(w)
	}
	if i.Expr != nil {
		io.WriteString(w, "(")
		i.Expr.format(w)
		io.WriteString(w, ")")
	}
	if i.Func != nil {
		i.Func.format(w)
	}
}

func (p *boolItem) evaluator(v *validator) boolEval {
	if p.Const != nil {
		return p.Const.evaluator(v)
	}
	if p.Expr != nil {
		return p.Expr.evaluator(v)
	}
	if p.Func != nil {
		return p.Func.evaluator(v)
	}
	return nil
}

type boolConst struct {
	Value string `parser:"@( 'TRUE' | 'FALSE' )"`
}

func (c *boolConst) format(w io.Writer) {
	io.WriteString(w, c.Value)
}

func (c *boolConst) evaluator(v *validator) boolEval {
	value := c.Value == "TRUE"
	return func(f failure) bool {
		return value
	}
}

type boolFunction struct {
	Function string        `parser:"@Ident"`
	Args     []*stringExpr `parser:"'(' @@ ( ',' @@ )* ')'"`
}

func (f *boolFunction) format(w io.Writer) {
	io.WriteString(w, f.Function)
	io.WriteString(w, "(")
	for i, arg := range f.Args {
		if i > 0 {
			io.WriteString(w, ", ")
		}
		arg.format(w)
	}
	io.WriteString(w, ")")
}

func (f *boolFunction) evaluator(v *validator) boolEval {
	switch strings.ToLower(f.Function) {
	case "regexp_contains":
		if len(f.Args) != 2 {
			v.reportError(fmt.Errorf("invalid number of arguments to REGEXP_CONTAINS: got %v, want 2", len(f.Args)))
			return nil
		}
		valueEval := f.Args[0].evaluator(v)
		pattern, ok := f.Args[1].asConstant(v)
		if !ok {
			// For efficiency reasons, we require the second argument to be a
			// constant so that we can pre-compile the regular expression.
			v.reportError(fmt.Errorf("expected second argument to REGEXP_CONTAINS to be a constant pattern"))
			return nil
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			v.reportError(fmt.Errorf("invalid regular expression %q", pattern))
			return nil
		}

		return func(f failure) bool {
			value := valueEval(f)
			return re.MatchString(value)
		}
	default:
		v.reportError(fmt.Errorf("undefined function: %q", f.Function))
		return nil
	}
}

type boolPredicate struct {
	Value *stringExpr `parser:"@@"`
	Test  *boolTest   `parser:"@@"`
}

func (t *boolPredicate) format(w io.Writer) {
	t.Value.format(w)
	t.Test.format(w)
}

func (t *boolPredicate) evaluator(v *validator) boolEval {
	value := t.Value.evaluator(v)
	test := t.Test.evaluator(v)
	return func(f failure) bool {
		return test(f, value(f))
	}
}

type boolTest struct {
	Comp      *compPredicate      `parser:"@@"`
	Negatable *negatablePredicate `parser:"| @@"`
}

func (t *boolTest) format(w io.Writer) {
	if t.Comp != nil {
		t.Comp.format(w)
	}
	if t.Negatable != nil {
		t.Negatable.format(w)
	}
}

func (t *boolTest) evaluator(v *validator) predicateEval {
	if t.Comp != nil {
		return t.Comp.evaluator(v)
	}
	return t.Negatable.evaluator(v)
}

type negatablePredicate struct {
	Not  bool           `parser:"( @'NOT' )?"`
	In   *inPredicate   `parser:"( @@"`
	Like *likePredicate `parser:"| @@ )"`
}

func (p *negatablePredicate) format(w io.Writer) {
	if p.Not {
		io.WriteString(w, " NOT")
	}
	if p.In != nil {
		p.In.format(w)
	}
	if p.Like != nil {
		p.Like.format(w)
	}
}

func (p *negatablePredicate) evaluator(v *validator) predicateEval {
	var predicate predicateEval
	if p.In != nil {
		predicate = p.In.evaluator(v)
	}
	if p.Like != nil {
		predicate = p.Like.evaluator(v)
	}
	if p.Not {
		return func(f failure, s string) bool {
			return !predicate(f, s)
		}
	}
	return predicate
}

type compPredicate struct {
	Op    string      `parser:"@( '=' | '!=' | '<>' )"`
	Value *stringExpr `parser:"@@"`
}

func (p *compPredicate) format(w io.Writer) {
	fmt.Fprintf(w, " %s ", p.Op)
	p.Value.format(w)
}

func (p *compPredicate) evaluator(v *validator) predicateEval {
	val := p.Value.evaluator(v)
	switch p.Op {
	case "=":
		return func(f failure, s string) bool {
			return s == val(f)
		}
	case "!=", "<>":
		return func(f failure, s string) bool {
			return s != val(f)
		}
	default:
		panic("invalid op")
	}
}

type inPredicate struct {
	List []*stringExpr `parser:"'IN' '(' @@ ( ',' @@ )* ')'"`
}

func (p *inPredicate) format(w io.Writer) {
	io.WriteString(w, " IN (")
	for i, v := range p.List {
		if i > 0 {
			io.WriteString(w, ", ")
		}
		v.format(w)
	}
	io.WriteString(w, ")")
}

func (p *inPredicate) evaluator(v *validator) predicateEval {
	var list []stringEval
	for _, item := range p.List {
		list = append(list, item.evaluator(v))
	}
	return func(f failure, s string) bool {
		for _, item := range list {
			if item(f) == s {
				return true
			}
		}
		return false
	}
}

type likePredicate struct {
	Pattern *string `parser:"'LIKE' @String"`
}

func (p *likePredicate) format(w io.Writer) {
	io.WriteString(w, " LIKE ")
	io.WriteString(w, *p.Pattern)
}

func (p *likePredicate) evaluator(v *validator) predicateEval {
	likePattern, err := unescapeStringLiteral(*p.Pattern)
	if err != nil {
		v.reportError(err)
		return nil
	}

	// Rewrite the LIKE syntax in terms of a regular expression syntax.
	regexpPattern, err := likePatternToRegexp(likePattern)
	if err != nil {
		v.reportError(err)
		return nil
	}

	re, err := regexp.Compile(regexpPattern)
	if err != nil {
		v.reportError(fmt.Errorf("invalid LIKE expression: %s", likePattern))
		return nil
	}
	return func(f failure, s string) bool {
		return re.MatchString(s)
	}
}

type stringExpr struct {
	Literal *string `parser:"@String"`
	Ident   *string `parser:"| @Ident"`
}

func (e *stringExpr) format(w io.Writer) {
	if e.Literal != nil {
		io.WriteString(w, *e.Literal)
	}
	if e.Ident != nil {
		io.WriteString(w, *e.Ident)
	}
}

// asConstant attempts to evaluate stringExpr as a compile-time constant.
// Returns the string value (assuming it is valid and constant) and
// whether it is a constant.
func (e *stringExpr) asConstant(v *validator) (value string, ok bool) {
	if e.Literal != nil {
		literal, err := unescapeStringLiteral(*e.Literal)
		if err != nil {
			v.reportError(err)
			return "", true
		}
		return literal, true
	}
	return "", false
}

func (e *stringExpr) evaluator(v *validator) stringEval {
	if e.Literal != nil {
		literal, err := unescapeStringLiteral(*e.Literal)
		if err != nil {
			v.reportError(err)
			return nil
		}
		return func(f failure) string { return literal }
	}
	if e.Ident != nil {
		varName := *e.Ident
		var accessor func(c *clustering.Failure) string
		switch varName {
		case "test":
			accessor = func(f *clustering.Failure) string {
				return f.TestID
			}
		case "reason":
			accessor = func(f *clustering.Failure) string {
				return f.Reason.GetPrimaryErrorMessage()
			}
		default:
			v.reportError(fmt.Errorf("undeclared identifier %q", varName))
		}
		return func(f failure) string { return accessor(f) }
	}
	return nil
}

var (
	lex = lexer.MustSimple([]lexer.Rule{
		{Name: "whitespace", Pattern: `\s+`, Action: nil},
		{Name: "Keyword", Pattern: `(?i)(TRUE|FALSE|AND|OR|NOT|LIKE|IN)\b`, Action: nil},
		{Name: "Ident", Pattern: `([a-zA-Z_][a-zA-Z0-9_]*)\b`, Action: nil},
		{Name: "String", Pattern: stringLiteralPattern, Action: nil},
		{Name: "Operators", Pattern: `!=|<>|[,()=]`, Action: nil},
	})

	parser = participle.MustBuild(
		&boolExpr{},
		participle.Lexer(lex),
		participle.Upper("Keyword"),
		participle.Map(lowerMapper, "Ident"),
		participle.CaseInsensitive("Keyword"))
)

func lowerMapper(token lexer.Token) (lexer.Token, error) {
	token.Value = strings.ToLower(token.Value)
	return token, nil
}

// Parse parses a failure association rule from the specified text.
// idents is the set of identifiers that are recognised by the application.
func Parse(text string) (*Expr, error) {
	expr := &boolExpr{}
	if err := parser.ParseString("", text, expr); err != nil {
		return nil, errors.Annotate(err, "syntax error").Err()
	}

	v := newValidator()
	eval := expr.evaluator(v)
	if err := v.error(); err != nil {
		return nil, err
	}
	return &Expr{
		expr: expr,
		eval: eval,
	}, nil
}
