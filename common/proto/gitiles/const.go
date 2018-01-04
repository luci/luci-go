package gitiles

// These constants are possible values for RefsRequest.RefsPath field.
// Not an exhaustive list.
const (
	// AllRefs instructs the client to fetch all refs.
	AllRefs = "refs"
	// Branches instructs the client to fetch all branches.
	Branches = "refs/heads"
	// Tags instructs the client to fetch all tags.
	Tags = "refs/tags"
)
