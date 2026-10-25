package main

import (
	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/driver/quadlet"
	"github.com/Moq77111113/vessel/internal/site"
	"github.com/Moq77111113/vessel/pkg/driver"
)

var machines = []driver.Driver{
	quadlet.New(machineRoot, quadlet.Exec),
}

const binary = "/proc/self/exe"

const machineRoot = "/"

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
