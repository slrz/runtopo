package runner

import (
	"context"

	"slrz.net/runtopo/topology"
)

// The Runner interface describes the capability to simulate a network
// topology.
type Runner interface {
	// Run starts up the provided topology or returns an error.  When the
	// context is canceled, implementations ought to make an effort to
	// clean up and release any previously acquired resources.
	Run(context.Context, *topology.T) error
}
