// Command vessel builds a delivery in CI, and installs it on the machine once packed.
package main

import (
	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/driver/quadlet"
	"github.com/Moq77111113/vessel/internal/site"
	"github.com/Moq77111113/vessel/pkg/driver"
)

// machines is the one place that names a concrete deployment kind. Pick tries them in order.
var machines = []driver.Machine{
	quadlet.New(quadlet.Exec),
}

// binary reads the running binary, even after the path it was started from names another file.
const binary = "/proc/self/exe"

// machineRoot is where a verb reads and writes: this machine, never a directory beside it.
const machineRoot = "/"

// New returns the command tree this binary offers. A binary packed with a bundle
// installs that bundle and cannot build another one.
func New() *cobra.Command {
	if site.CarriesABundle(binary) {
		return newPacked(binary)
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
	vessel.AddCommand(newBuild(), newInspect(), newInstallCommand(), newResumeCommand(), newUpgradeCommand(), newStatus(), newUninstall())
	return vessel
}
