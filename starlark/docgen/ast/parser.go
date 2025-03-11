// Copyright 2019 The LUCI Authors.
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

// Package ast defines AST relevant for the documentation generation.
//
// It recognizes top-level function declarations, top-level assignments (e.g.
// for constants and aliases), load(...) statements (to follow imported
// symbols), and struct(...) declarations.
package ast

import (
	"fmt"
	"strings"

	"go.starlark.net/syntax"
)

// Ellipsis represents a complex expression that we don't care about.
//
// A value of Ellipsis type is usually literally just "...".
type Ellipsis string

// Node is a documentation-relevant declaration of something in a file.
//
// Nodes form a tree. This tree is a reduction of a full AST of the starlark
// file to a form we care about when generating the documentation.
//
// The top of the tree is represented by a Module node.
type Node interface {
	// Name is the name of the entity this node defines.
	//
	// E.g. it's the name of a function, variable, constant, etc.
	//
	// It may be a "private" name. Many definitions are defined using their
	// private names first, and then exposed publicly via separate definition
	// (such definitions are represented by Reference or ExternalReference nodes).
	Name() string

	// Span is where this node was defined in the original starlark code.
	Span() (start syntax.Position, end syntax.Position)

	// Comments is a comment block immediately preceding the definition.
	Comments() string

	// Doc is a documentation string for this symbol extracted either from a
	// docstring or from comments.
	Doc() string

	// populateFromAST sets the fields based on the given starlark AST node.
	populateFromAST(name string, n syntax.Node)
}

// EnumerableNode is a node that has a variable number of subnodes.
//
// Used to represents structs, modules and invocations.
type EnumerableNode interface {
	Node

	// EnumNodes returns a list of subnodes. It should not be mutated.
	EnumNodes() []Node
}

// base is embedded by all node types and implements some Node methods for them.
//
// It carries name of the node, where it is defined, and surrounding comments.
type base struct {
	name string
	ast  syntax.Node // where it was defined in Starlark AST
}

func (b *base) Name() string                             { return b.name }
func (b *base) Span() (syntax.Position, syntax.Position) { return b.ast.Span() }

func (b *base) Comments() string {
	// Get all comments before `ast`. In particular if there are multiple comment
	// blocks separated by new lines, `before` contains all of them.
	var before []syntax.Comment
	if all := b.ast.Comments(); all != nil {
		before = all.Before
	}
	if len(before) == 0 {
		return ""
	}

	// Grab a line number where 'ast' itself is defined.
	start, _ := b.ast.Span()

	// Pick only comments immediately preceding this line.
	var comments []string
	for idx := len(before) - 1; idx >= 0; idx-- {
		if before[idx].Start.Line != start.Line-int32(len(comments))-1 {
			break // detected a skipped line, which indicates it's a different block
		}
		// Strip '#\s?' (but only one space, spaces may be significant for the doc
		// syntax in the comment).
		line := strings.TrimPrefix(strings.TrimPrefix(before[idx].Text, "#"), " ")
		comments = append(comments, line)
	}

	// Reverse 'comments', since we recorded them in reverse order.
	for l, r := 0, len(comments)-1; l < r; l, r = l+1, r-1 {
		comments[l], comments[r] = comments[r], comments[l]
	}
	return strings.Join(comments, "\n")
}

// Doc extracts the documentation for the symbol from its comments.
func (b *base) Doc() string {
	return b.Comments()
}

func (b *base) populateFromAST(name string, ast syntax.Node) {
	b.name = name
	b.ast = ast
}

// Var is a node that represents '<var> = int|string|<expr>' definition.
//
// This is a "terminal" definition, not a reference to something defined
// elsewhere. Usually a constant or some computation we replace with '...' in
// the docs.
type Var struct {
	base

	Value any // string | int64 | *big.Int | Ellipsis
}

// Function is a node that represents a function definition.
type Function struct {
	base

	docstring string // a doc string, if any
}

// Doc extracts the documentation from the docstring.
func (n *Function) Doc() string { return n.docstring }

// Reference is a node that represents <var> = a.b.c.
//
// It is either a top-level assignment, or a keyword argument in a function call
// (e.g. when defining struct(...)).
type Reference struct {
	base

	Path []string // the ref path on the right hand side, e.g. ['a', 'b', 'c'].
}

// ExternalReference is a node that represents a symbol imported though
// load(...) statement.
//
// For load statement load("file.star", x="y") we get an ExternalReference with
// name "x", ExternalName "y" and Module "file.star".
type ExternalReference struct {
	base

	ExternalName string // name of the symbol in the loaded module
	Module       string // normalized path of the loaded module
}

// Invocation represents `<name> = ns1.ns2.func(arg1=..., arg2=...)` call. Only
// keyword arguments are recognized.
type Invocation struct {
	base

	Func []string // e.g. ["ns1, "ns2", "func"]
	Args []Node   // keyword arguments in order of their definition
}

// EnumNodes returns list of nodes that represent arguments.
func (inv *Invocation) EnumNodes() []Node { return inv.Args }

// Namespace is a node that contains a bunch of definitions grouped together.
//
// Examples of namespaces are top-level module dicts and structs.
type Namespace struct {
	base

	Nodes []Node // nodes defined in the namespace, in order they were defined
}

// EnumNodes returns list of nodes that represent definitions in the namespace.
func (ns *Namespace) EnumNodes() []Node { return ns.Nodes }

// Module is a parsed Starlark file.
type Module struct {
	Namespace // all top-level symbols

	docstring string // a doc string, if any
}

// Doc extracts the documentation from the docstring.
func (n *Module) Doc() string { return n.docstring }

// ParseModule parses a single Starlark module.
//
// Filename is only used when recording position information.
func ParseModule(opts *syntax.FileOptions, filename, body string, normalize func(string) (string, error)) (*Module, error) {
	ast, err := opts.Parse(filename, body, syntax.RetainComments)
	if err != nil {
		return nil, err
	}

	m := &Module{docstring: extractDocstring(ast.Stmts)}
	m.populateFromAST(filename, ast)

	// emit adds a node to the module.
	emit := func(name string, ast syntax.Node, n Node) {
		n.populateFromAST(name, ast)
		m.Nodes = append(m.Nodes, n)
	}

	// Walk over top-level statements and match them against patterns we recognize
	// as relevant.
	for _, stmt := range ast.Stmts {
		switch st := stmt.(type) {
		case *syntax.LoadStmt:
			// A load(...) statement. Each imported symbol ends up in the module's
			// namespace, so add corresponding ExternalReference nodes.
			s := st.Module.Value.(string)
			if s, err = normalize(s); err != nil {
				return nil, fmt.Errorf("load() statement invalid: %w", err)
			}
			for i, nm := range st.To {
				emit(nm.Name, st, &ExternalReference{
					ExternalName: st.From[i].Name,
					Module:       s,
				})
			}

		case *syntax.DefStmt:
			// A function declaration: "def name(...)".
			emit(st.Name.Name, st, &Function{
				docstring: extractDocstring(st.Body),
			})

		case *syntax.AssignStmt:
			// A top level assignment. We care only about <var> = ... (i.e. when LHS
			// is a simple variable, not a tuple or anything like that).
			if st.Op != syntax.EQ {
				continue
			}
			lhs := matchSingleIdent(st.LHS)
			if lhs == "" {
				continue
			}
			if n := parseAssignmentRHS(st.RHS); n != nil {
				emit(lhs, st, n)
			}
		}
	}

	return m, nil
}

// parseAssignmentRHS parses RHS of statements like "<var> = <expr>".
//
// Name of the returned node and Star/End/Comments should be populated by the
// caller.
//
// Only the following forms are recognized:
//
//	Var: <var> = <literal>|<complex expr>
//	Reference: <var> = <var>[.<field>]*
//	Namespace: <var> = struct(...)
func parseAssignmentRHS(rhs syntax.Expr) Node {
	// <var> = <literal>.
	if literal := matchSingleLiteral(rhs); literal != nil {
		return &Var{Value: literal}
	}

	// <var> = <var>[.<field>]*.
	if path := matchRefPath(rhs); path != nil {
		return &Reference{Path: path}
	}

	// <var> = <fn>(...).
	if fn, args := matchSimpleCall(rhs); len(fn) != 0 {
		// Pick all 'k=v' pairs from args and parse them as assignments.
		var nodes []Node
		for _, arg := range args {
			if lhs, rhs := matchEqExpr(arg); lhs != "" {
				if n := parseAssignmentRHS(rhs); n != nil {
					n.populateFromAST(lhs, arg)
					nodes = append(nodes, n)
				}
			}
		}

		// <var> = struct(...).
		if len(fn) == 1 && fn[0] == "struct" {
			return &Namespace{Nodes: nodes}
		}

		// <var> = ns.ns.func(arg1=..., arg2=...).
		return &Invocation{Func: fn, Args: nodes}
	}

	// <var> = <expr>.
	return &Var{Value: Ellipsis("...")}
}

// extractDocstring returns a doc string for the given body.
//
// A docstring is a string literal that comes first in the body, if any.
func extractDocstring(body []syntax.Stmt) string {
	if len(body) == 0 {
		return ""
	}
	expr, ok := body[0].(*syntax.ExprStmt)
	if !ok {
		return ""
	}
	literal, ok := expr.X.(*syntax.Literal)
	if !ok || literal.Token != syntax.STRING {
		return ""
	}
	return literal.Value.(string)
}

// matchSingleIdent matches an <Expr> to <Ident>, returning ident's name.
func matchSingleIdent(expr syntax.Expr) string {
	if ident, ok := expr.(*syntax.Ident); ok {
		return ident.Name
	}
	return ""
}

// matchSingleLiteral matches an <Expr> to <Literal>, returning literal's value.
//
// The returned value is string | int64 | *big.Int.
func matchSingleLiteral(expr syntax.Expr) any {
	if literal, ok := expr.(*syntax.Literal); ok {
		return literal.Value
	}
	return nil
}

// matchRefPath matches an <Expr> to <Ident>(.<Ident>)* returning identifier'
// names as a list of strings.
func matchRefPath(expr syntax.Expr) (path []string) {
loop:
	for {
		switch next := expr.(type) {
		case *syntax.DotExpr: // next in chain
			path = append(path, next.Name.Name)
			expr = next.X
		case *syntax.Ident: // last in chain
			path = append(path, next.Name)
			break loop
		default:
			return nil // not a simple ref path, has additional structure, give up
		}
	}
	// Expr "a.b.c" results in ['c', 'b', 'a'], reverse.
	for l, r := 0, len(path)-1; l < r; l, r = l+1, r-1 {
		path[l], path[r] = path[r], path[l]
	}
	return
}

// matchSimpleCall matches an <Expr> to <Ident>(.<Ident>)*(<Expr>*), returning
// them.
func matchSimpleCall(expr syntax.Expr) (fn []string, args []syntax.Expr) {
	call, ok := expr.(*syntax.CallExpr)
	if !ok {
		return nil, nil
	}
	if fn = matchRefPath(call.Fn); len(fn) == 0 {
		return nil, nil
	}
	return fn, call.Args
}

// matchEqExpr matches an <Expr> to <Ident>=<Expr>, returning them.
func matchEqExpr(expr syntax.Expr) (lhs string, rhs syntax.Expr) {
	bin, ok := expr.(*syntax.BinaryExpr)
	if !ok || bin.Op != syntax.EQ {
		return "", nil
	}
	if lhs = matchSingleIdent(bin.X); lhs == "" {
		return "", nil
	}
	return lhs, bin.Y
}
