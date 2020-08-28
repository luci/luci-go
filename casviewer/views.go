package casviewer

import (
	"fmt"
	"net/http"

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"go.chromium.org/luci/server/router"
)

// renderNotFound renders 400 BadRequest page.
func renderBadRequest(c *router.Context, errMsg string) {
	// TODO(crbug.com/1121471): render 404 html.
	m := fmt.Sprintf("Error: Bad Request. %s", errMsg)
	http.Error(c.Writer, m, http.StatusBadRequest)
}

// renderNotFound renders 404 NotFound page.
func renderNotFound(c *router.Context) {
	// TODO(crbug.com/1121471): render 400 html.
	http.Error(c.Writer, "Error: Not Found", http.StatusNotFound)
}

// renderInternalServerError renders 500 InternalServerError page.
func renderInternalServerError(c *router.Context, errMsg string) {
	// TODO(crbug.com/1121471): render 500 html.
	m := fmt.Sprintf("Error: %s", errMsg)
	http.Error(c.Writer, m, http.StatusInternalServerError)
}

// renderDirectory renders a directory page.
func renderDirectory(c *router.Context, d *repb.Directory) {
	// TODO(crbug.com/1121471): render html.

	dirs := d.GetDirectories()
	c.Writer.Write([]byte(fmt.Sprintf("dirs: %v\n", dirs)))
	files := d.GetFiles()
	c.Writer.Write([]byte(fmt.Sprintf("files: %v\n", files)))
	links := d.GetSymlinks()
	c.Writer.Write([]byte(fmt.Sprintf("symlinks %v\n", links)))
}
