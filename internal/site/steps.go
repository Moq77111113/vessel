package site

import (
	"errors"
	"time"

	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
)

type steps struct {
	records *release.Records
	opening release.Record
	next    release.Record
}

func openSteps(before state, next release.Record) (*steps, error) {
	opening := next
	opening.Files = release.Union(before.current.Files, next.Files)
	opening.Services = unionServices(before.current.Services, next.Services)
	opening.Steps = before.progress.Steps
	opening.Actions = before.progress.Actions
	return &steps{records: before.records, opening: opening, next: next}, before.records.Write(opening)
}

func (s *steps) run(step release.Step, do func() error) error {
	if s.opening.Has(step) {
		return nil
	}
	if err := do(); err != nil {
		return err
	}
	s.opening.Steps = append(s.opening.Steps, step)
	return s.records.Write(s.opening)
}

func (s *steps) action(step release.Step, command string, do func() error) error {
	s.opening.Actions = append(s.opening.Actions, release.Action{Command: command, Step: step})
	if err := s.records.Write(s.opening); err != nil {
		return err
	}
	last := len(s.opening.Actions) - 1
	err := do()
	if errors.Is(err, disk.ErrKilled) {
		return err
	}
	if err != nil && !errors.Is(err, disk.ErrOutput) {
		s.opening.Actions = s.opening.Actions[:last]
		return errors.Join(err, s.records.Write(s.opening))
	}
	s.opening.Actions[last].End = time.Now().UTC()
	return errors.Join(err, s.records.Write(s.opening))
}

func count(actions []release.Action, step release.Step) int {
	n := 0
	for _, action := range actions {
		if action.Step == step {
			n++
		}
	}
	return n
}

func (s *steps) close() (release.Record, error) {
	s.next.Steps = s.opening.Steps
	s.next.Actions = s.opening.Actions
	s.next.End = time.Now().UTC()
	return s.next, s.records.Write(s.next)
}
