package access

import (
	"regexp"

	google "github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

var (
	rgxActionID = regexp.MustCompile(`^[A-Z_]+$`)
	rgxRoleID   = rgxActionID
	rgxKind     = regexp.MustCompile(`^[a-z\-]+$`)
	rgxParam    = regexp.MustCompile(`^\{[a-z\-]+}$`)
)

// Resource is an access-controlled resource.
type Resource struct {
	// Description is included in the NewServer's Access.Description response.
	Description DescriptionResponse_ResourceDescription
	// PermittedActions is called by NewServer's Access.PermittedActions
	// implementation after resolving the resource.
	PermittedActions func(ctx context.Context, args []string) (*PermittedActionsResponse, error)

	pattern      []patternTuple
	validActions stringset.Set
}

func (r *Resource) parseDescription() error {
	p, err := parsePattern(r.Description.Pattern)
	if err != nil {
		return errors.Annotate(err, "invalid pattern").Err()
	}
	r.pattern = p

	r.validActions = stringset.New(len(r.Description.Actions))
	for _, a := range r.Description.Actions {
		r.validActions.Add(a.ActionId)
	}
	return nil
}

// validate returns an error if Resource.Description is invalid.
// Initializes r.pattern.
func (r *Resource) validate() error {
	if err := r.parseDescription(); err != nil {
		return err
	}
	return r.Description.validate(r.pattern)
}

type server struct {
	desc        *DescriptionResponse
	patternTree *patternNode
}

// NewServer validates description and creates an AccessServer.
func NewServer(resources []*Resource) (AccessServer, error) {
	desc := &DescriptionResponse{
		Resources: make([]*DescriptionResponse_ResourceDescription, len(resources)),
	}

	tree := &patternNode{}
	for i, r := range resources {
		err := r.validate()
		if err != nil {
			return nil, errors.Annotate(err, "invalid resource #%d", i).Err()
		}
		if err := tree.add(r); err != nil {
			return nil, err
		}
		desc.Resources[i] = &r.Description
	}

	return &server{
		desc:        desc,
		patternTree: tree,
	}, nil
}

func (s *server) Description(context.Context, *google.Empty) (*DescriptionResponse, error) {
	return s.desc, nil
}

func (s *server) PermittedActions(c context.Context, req *PermittedActionsRequest) (*PermittedActionsResponse, error) {
	resource, args, err := s.patternTree.resolve(req.Resource)
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	res, err := resource.PermittedActions(c, args)
	if err != nil {
		return nil, err
	}

	for _, a := range res.Actions {
		if !resource.validActions.Has(a) {
			return nil, grpc.Errorf(codes.Internal, "PermittedActions callback returned an undefined action %q", a)
		}
	}
	return res, nil
}
