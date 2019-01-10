package commands

import (
	"github.com/concourse/concourse/fly/commands/internal/flaghelpers"
)

type PinResourceVersionCommand struct {
	Pipeline flaghelpers.PipelineFlag `short:"prv"  long:"pin-resource-version" required:"true" description""`
}
