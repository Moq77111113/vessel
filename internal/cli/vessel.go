// Package cli is the composition root: it builds every concrete type and wires the commands.
package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/installer"
	"github.com/Moq77111113/vessel/internal/quadlet"
)

// readers is the one place that names a concrete format. Pick tries them in this order.
var readers = []descriptor.Reader{
	quadlet.NewReader(),
}

// New returns the command tree this binary offers. A binary packed with a bundle
// installs that bundle and cannot build another one.
func New() *cobra.Command {
	if self, err := os.Executable(); err == nil && installer.CarriesABundle(self) {
		return newPacked(self)
	}
	return newVessel()
}

func newVessel() *cobra.Command {
	vessel := &cobra.Command{
		Use:           "vessel",
		Short:         "A deployment linker for machines with no network",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	vessel.AddCommand(newLink(), newPack(), newLoad(), newInspect())
	return vessel
}
