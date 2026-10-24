package site

import (
	"context"
	"fmt"
	"io"

	"github.com/Moq77111113/vessel/internal/site/release"
)

type List struct {
	Root string
}

type ListRequest struct {
	Out io.Writer
}

type ListResult struct {
	Records []release.Record
}

func (l List) Run(_ context.Context, req ListRequest) (ListResult, error) {
	names, err := release.Deliveries(l.Root)
	if err != nil {
		return ListResult{}, err
	}
	if len(names) == 0 {
		fmt.Fprintln(req.Out, "this machine holds no delivery")
		return ListResult{}, nil
	}
	var result ListResult
	for _, name := range names {
		records, err := openRecords(l.Root, name)
		if err != nil {
			return ListResult{}, err
		}
		record, err := lastRecord(records, name)
		if err != nil {
			return ListResult{}, err
		}
		fmt.Fprintf(req.Out, "%s %s\n", record.Name, record.Version)
		result.Records = append(result.Records, record)
	}
	return result, nil
}
