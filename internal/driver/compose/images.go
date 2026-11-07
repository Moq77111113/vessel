package compose

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

var (
	ErrBuild         = errors.New("a service builds its image, vessel only carries images")
	ErrNoImage       = errors.New("a service names no image")
	ErrImageVariable = errors.New("an image is named through a variable, pin it in the compose file")
	ErrImageLocation = errors.New("an image is not a plain or quoted scalar vessel can patch in place")
)

func imageRelocations(index int, data []byte, file project) ([]delivery.Relocation, error) {
	starts := lineStarts(data)
	var relocs []delivery.Relocation
	for name, svc := range file.Services {
		if svc.Build.Kind != 0 {
			return nil, fmt.Errorf("%s: %w", name, ErrBuild)
		}
		if svc.Image.Kind == 0 {
			return nil, fmt.Errorf("%s: %w", name, ErrNoImage)
		}
		image := resolve(&svc.Image)
		if strings.Contains(image.Value, "$") {
			return nil, fmt.Errorf("%s: %q: %w", name, image.Value, ErrImageVariable)
		}
		offset, err := locate(data, starts, image)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if slices.ContainsFunc(relocs, func(r delivery.Relocation) bool { return r.Offset == offset }) {
			continue
		}
		ref, err := delivery.ParseRef(image.Value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		relocs = append(relocs, delivery.Relocation{File: index, Offset: offset, Length: len(image.Value), Ref: ref})
	}
	slices.SortFunc(relocs, func(a, b delivery.Relocation) int { return a.Offset - b.Offset })
	return relocs, nil
}

func lineStarts(data []byte) []int {
	starts := []int{0}
	for i, b := range data {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// yaml.v3 counts columns in runes from 1, and starts an anchored scalar at its anchor.
func locate(data []byte, starts []int, node *yaml.Node) (int, error) {
	if node.Line < 1 || node.Line > len(starts) {
		return 0, fmt.Errorf("%q: %w", node.Value, ErrImageLocation)
	}
	offset := starts[node.Line-1]
	for range node.Column - 1 {
		_, size := utf8.DecodeRune(data[offset:])
		offset += size
	}
	if node.Anchor != "" {
		offset += len("&" + node.Anchor)
		offset += len(data[offset:]) - len(bytes.TrimLeft(data[offset:], " \t"))
	}
	if node.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle) != 0 {
		offset++
	}
	end := offset + len(node.Value)
	if end > len(data) || string(data[offset:end]) != node.Value {
		return 0, fmt.Errorf("%q: %w", node.Value, ErrImageLocation)
	}
	return offset, nil
}
