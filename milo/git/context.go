package git

import "golang.org/x/net/context"

var luciProjectKey = "luci_project_ctx"

// WithProject annotates the context object with the LUCI project that
// a request is handled for.
func WithProject(ctx context.Context, project string) context.Context {
	return context.WithValue(ctx, &luciProjectKey, project)
}

// ProjectFromContext is the opposite of WithProject, is extracts the
// LUCI project which the current call stack is handling a request for.
func ProjectFromContext(ctx context.Context) string {
	project, ok := ctx.Value(luciProjectKey).(string)
	if !ok {
		panic("LUCI project not available in context")
	}
	return project
}
