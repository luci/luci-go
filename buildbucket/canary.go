package buildbucket

import "go.chromium.org/luci/common/errors"

// CanaryPreference controls if the canary infrastructure is allowed,
// required, or forbidden for a build.
type CanaryPreference int

const (
	// CanaryAllowed specifies that it is up to the infrastructure
	// to decide if canary will be used.
	CanaryAllowed CanaryPreference = iota
	// CanaryForbidden specifies that the prod version of infrastructure
	// must be used.
	CanaryForbidden
	// CanaryRequired specifies that the canary version of build infrastructure
	// must be used. If the canary infrastructure does not exist, a build
	// creation request will fail.
	CanaryRequired
)

func (c CanaryPreference) endpointsString() (string, error) {
	switch c {
	case CanaryAllowed:
		return "AUTO", nil
	case CanaryForbidden:
		return "PROD", nil
	case CanaryRequired:
		return "CANARY", nil
	default:
		return "", errors.Reason("invalid value of canary preference %q", c).Err()
	}
}

func parseEndpointsCanaryPreference(s string) (CanaryPreference, error) {
	switch s {
	case "AUTO":
		return CanaryAllowed, nil
	case "PROD":
		return CanaryForbidden, nil
	case "CANARY":
		return CanaryRequired, nil
	default:
		return 0, errors.Reason("invalid value of canary preference %q", s).Err()
	}
}
