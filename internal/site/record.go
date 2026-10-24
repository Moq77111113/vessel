package site

import (
	"errors"
	"fmt"

	"github.com/Moq77111113/vessel/internal/site/release"
)

func openRecords(root, name string) (*release.Records, error) {
	dir, err := release.Dir(root, name)
	if err != nil {
		return nil, err
	}
	return release.NewRecords(dir), nil
}

func lastRecord(records *release.Records, name string) (release.Record, error) {
	record, err := records.Read()
	if errors.Is(err, release.ErrNoRecord) {
		return release.Record{}, fmt.Errorf("%s: %w", name, err)
	}
	return record, err
}
