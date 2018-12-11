package frontend

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/router"
)

// handleLUCIBuild renders a LUCI build.
func handleLUCIBuild(c *router.Context) error {
	bucket := c.Params.ByName("bucket")
	builder := c.Params.ByName("builder")
	numberOrId := c.Params.ByName("numberOrId")

	var address string
	if strings.HasPrefix(numberOrId, "b") {
		address = numberOrId[1:]
	} else {
		address = fmt.Sprintf("%s/%s/%s", bucket, builder, numberOrId)
	}

	build, err := buildbucket.GetBuild(c.Context, address, true)
	// TODO(nodir): after switching to API v2, check that project, bucket
	// and builder in parameters indeed match the returned build. This is
	// relevant when the build is loaded by id.
	return renderBuildLegacy(c, build, err)
}

// redirectLUCIBuild redirects to a canonical build URL
// e.g. to /p/{project}/builders/{bucket}/{builder}/{number or id}.
func redirectLUCIBuild(c *router.Context) error {
	idStr := c.Params.ByName("id")
	// Verify it is an int64.
	if _, err := strconv.ParseInt(idStr, 10, 64); err != nil {
		return errors.Annotate(err, "invalid id").Tag(common.CodeParameterError).Err()
	}

	build, err := buildbucket.GetRawBuild(c.Context, idStr)
	if err != nil {
		return err
	}

	// If the build has a number, redirect to a URL with it.
	builder := ""
	u := *c.Request.URL
	for _, t := range build.Tags {
		switch k, v := strpair.Parse(t); k {
		case bbv1.TagBuildAddress:
			_, project, bucket, builder, number, _ := bbv1.ParseBuildAddress(v)
			if number > 0 {
				u.Path = fmt.Sprintf("/p/%s/builders/%s/%s/%d", project, bucket, builder, number)
				http.Redirect(c.Writer, c.Request, u.String(), http.StatusMovedPermanently)
				return nil
			}

		case bbv1.TagBuilder:
			builder = v
		}
	}
	if builder == "" {
		return errors.Reason("build %s does not have a builder", idStr).Tag(common.CodeParameterError).Err()
	}

	u.Path = fmt.Sprintf("/p/%s/builders/%s/%s/b%d", build.Project, build.Bucket, builder, build.Id)
	http.Redirect(c.Writer, c.Request, u.String(), http.StatusMovedPermanently)
	return nil
}
