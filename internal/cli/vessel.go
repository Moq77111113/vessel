// Package cli is the composition root: it builds every concrete type and wires the commands.
package cli

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/installer"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
)

// machines is the one place that names a concrete deployment kind. Pick tries them in order.
var machines = []machine.Machine{
	quadlet.New(machine.Exec),
}

// machineRoot is where a verb reads and writes: this machine, never a directory beside it.
const machineRoot = "/"

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
	vessel.AddCommand(newBuild(), newInspect(), newInstallCommand(), newUpgradeCommand(), newStatus(), newUninstall())
	return vessel
}
