package main

import (
	"errors"

	"github.com/Moq77111113/vessel/internal/site"
	"github.com/Moq77111113/vessel/pkg/driver"
)

func Code(err error) int {
	switch {
	case errors.Is(err, driver.ErrNotReady):
		return 3
	case errors.Is(err, site.ErrPartlyInstalled):
		return 5
	}
	return 4
}
