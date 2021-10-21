package appengine

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"os/exec"
	"path"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"
)

var statusAssign = &analysis.Analyzer{
	Name: "statusAssignment",
	Doc:  "reports direct assignments to Build.Status",
	Run: func(pass *analysis.Pass) (interface{}, error) {
		for _, file := range pass.Files {
			if strings.HasSuffix(pass.Fset.File(file.Pos()).Name(), "_test.go") {
				continue
			}

			ast.Inspect(file, func(n ast.Node) bool {
				if assn, ok := n.(*ast.AssignStmt); ok {
					checkAssignStmt(pass, assn)
				}
				return true
			})
		}

		return nil, nil
	},
}

const envKey = "BUILDBUCKET_RUN_STATUS_ASSIGN_CHECKER"

func init() {
	if os.Getenv(envKey) != "" {
		singlechecker.Main(statusAssign)
	}
}

func checkAssignStmt(pass *analysis.Pass, stmt *ast.AssignStmt) {
	for _, exp := range stmt.Lhs {
		ast.Inspect(exp, func(n ast.Node) bool {
			if se, ok := n.(*ast.SelectorExpr); ok {
				ot := pass.TypesInfo.TypeOf(se.X)
				if ot == nil {
					return true
				}
				pt, ok := ot.(*types.Pointer)
				if !ok {
					return true
				}
				nt, ok := pt.Elem().(*types.Named)
				if !ok {
					return true
				}
				typ := nt.Obj()

				if typ.Pkg().Path() != "go.chromium.org/luci/buildbucket/proto" || typ.Name() != "Build" {
					return true
				}
				// At this point we know the left part of the selector is a *Build

				switch se.Sel.Name {
				case "Status", "StartTime", "EndTime":
					pass.Report(analysis.Diagnostic{
						Pos:     stmt.Pos(),
						Message: fmt.Sprintf("Bare assignment to Build.%s; Use protoutil.SetStatus instead.", se.Sel.Name),
					})
				}
				return false
			}
			return true
		})
	}
}

func TestBuildStatusAssignment(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	_, curFile, _, _ := runtime.Caller(0)

	cmd := exec.Command(executable, "-c", "0", path.Dir(curFile)+"/...")
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, envKey+"=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	switch err := cmd.Run().(type) {
	case nil:
	case *exec.ExitError:
		t.Fail()

	default:
		t.Fatal(err)
	}
}
