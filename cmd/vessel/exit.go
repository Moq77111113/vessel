package main

import (
	"errors"

	"github.com/Moq77111113/vessel/internal/driver/quadlet"
	"github.com/Moq77111113/vessel/internal/site"
)

// Code maps an error to what a playbook reads: 3 the machine, 5 partway through an install, 4 otherwise.
func Code(err error) int {
	switch {
	case errors.Is(err, quadlet.ErrNotReady):
		return 3
	case errors.Is(err, site.ErrPartlyInstalled):
		return 5
	}
	// A failure that reached neither fact left the machine untouched.
	return 4
}
