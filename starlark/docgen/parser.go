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

package docgen

import (
	"go.starlark.net/syntax"
)

// Ellipsis represents a complex expressions that we don't care about.
type Ellipsis string

// Node is declaration of something in a starlark module.
//
// Nodes form a tree. This tree is a reduction of a full AST of the starlark
//file to structures we care about for the purpose of documentation generation.
//
// See Module for the root of this tree.
type Node interface {
	// Name is a name of the function, variable, constant, etc this node defines.
	//
	// It may be a "private" name. Many definitions are defined using they private
	// names first, and then exposed publicly via separate definition (such
	// definitions are represented by Reference or ExternalReference nodes).
	Name() string

	// Span is where this node was defined in the original starlark code.
	Span() (start syntax.Position, end syntax.Position)

	// Comments are comments preceding and surrounding the definition.
	Comments() *syntax.Comments

	// populateFromAST sets the fields based on the given AST node.
	populateFromAST(name string, n syntax.Node)
}

// base carries name of the node, where it is defined and surrounding comments.
//
// It is embedded by all node types. It also implements Node interface for them.
type base struct {
	name string      // name of the defined symbol, may be private
	ast  syntax.Node // where it was define in Starlark AST
}

func (b *base) Name() string                             { return b.name }
func (b *base) Span() (syntax.Position, syntax.Position) { return b.ast.Span() }
func (b *base) Comments() *syntax.Comments               { return b.ast.Comments() }
func (b *base) populateFromAST(name string, ast syntax.Node) {
	b.name = name
	b.ast = ast
}

// Var is a node that represented <var> = int|string|<expr> definition.
//
// This is a "terminal" definition, not a reference to something defined
// elsewhere.
type Var struct {
	base

	Value interface{} // string | int64 | *big.Int | Ellipsis
}

// Function is a node that represented a function definition.
type Function struct {
	base

	DocString string // a doc string, if any
}

// Reference is a node that represents <var> = a.b.c.
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

	ExternalName string
	Module       string
}

// Namespace is a node that contains a bunch of definitions grouped together.
//
// For example, a top-level dict of a Starlark module is a namespace. Structs
// are namespaces too.
type Namespace struct {
	base

	Nodes []Node // list of nodes defined in the namespace
}

// Module is a parsed Starlark file.
type Module struct {
	Namespace // all top-level symbols

	DocString string // a doc string, if any
}

// ParseModule parses a single Starlark module.
//
// Filename is only used when recording position information.
func ParseModule(filename, body string) (*Module, error) {
	ast, err := syntax.Parse(filename, body, syntax.RetainComments)
	if err != nil {
		return nil, err
	}

	m := &Module{DocString: extractDocString(ast.Stmts)}
	m.populateFromAST("", ast)

	// emit adds a node to the module.
	emit := func(name string, ast syntax.Node, n Node) {
		n.populateFromAST(name, ast)
		m.Nodes = append(m.Nodes, n)
	}

	for _, stmt := range ast.Stmts {
		switch st := stmt.(type) {

		case *syntax.DefStmt:
			// A function declaration: "def name(...)".
			emit(st.Name.Name, st, &Function{
				DocString: extractDocString(st.Body),
			})

		case *syntax.AssignStmt:
			// A top level assignment. We care only about <var> = ...
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

		case *syntax.LoadStmt:
			// A load(...) statement. Each imported symbol ends up in the module's
			// namespace, so add a corresponding ExternalReference node.
			for i, nm := range st.To {
				emit(nm.Name, st, &ExternalReference{
					ExternalName: st.From[i].Name,
					Module:       st.Module.Value.(string),
				})
			}
		}
	}

	return m, nil
}

// parseAssignmentRHS parses RHS of statements like "<var> = <expr>".
//
// Its name and Star/End/Comments should be populated by the caller.
//
// We are interested only in the following forms:
//   <var> = <literal>
//   <var> = <var>[.<field>]*
//   <var> = struct(...)
//   <var> = <expr>
func parseAssignmentRHS(rhs syntax.Expr) Node {
	// <var> = <literal>.
	if literal := matchSingleLiteral(rhs); literal != nil {
		return &Var{Value: literal}
	}

	// <var> = <var>[.<field>]*.
	if path := matchRefPath(rhs); path != nil {
		return &Reference{Path: path}
	}

	// <var> = struct(...).
	if fn, args := matchSimpleCall(rhs); fn == "struct" {
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
		return &Namespace{Nodes: nodes}
	}

	// <var> = <expr>.
	return &Var{Value: Ellipsis("...")}
}

// extractDocString returns a doc string for the given body.
//
// A docstring is a string literal, if it comes first in the body.
func extractDocString(body []syntax.Stmt) string {
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
func matchSingleLiteral(expr syntax.Expr) interface{} {
	if literal, ok := expr.(*syntax.Literal); ok {
		return literal.Value
	}
	return nil
}

// matchRefPath matches an <Expr> to <Ident>(.<Ident>)* returning identifier's
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

// matchSimpleCall matches an <Expr> to <Ident>(<Expr>*), returning them.
func matchSimpleCall(expr syntax.Expr) (fn string, args []syntax.Expr) {
	call, ok := expr.(*syntax.CallExpr)
	if !ok {
		return "", nil
	}
	if fn = matchSingleIdent(call.Fn); fn == "" {
		return "", nil
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
