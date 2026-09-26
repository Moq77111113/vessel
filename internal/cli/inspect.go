package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/report"
)

func newInspect() *cobra.Command {
	var into string

	command := &cobra.Command{
		Use:   "inspect <bundle>",
		Short: "Print what a bundle carries, without touching anything",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return inspectBundle(command.OutOrStdout(), args[0], into)
		},
	}
	bindEvidenceFlag(command, &into)
	return command
}

// bindEvidenceFlag gives command the --evidence flag that writes the carried evidence into a directory.
func bindEvidenceFlag(command *cobra.Command, into *string) {
	command.Flags().StringVar(into, "evidence", "", "write every SBOM and report the bundle carries into this directory")
}

func inspectBundle(out io.Writer, dir, into string) error {
	artifact, err := bundle.Open(dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s %s, read by %s, resolved for %s\n",
		artifact.Config.Name, artifact.Config.Version, artifact.Config.Machine, artifact.Config.Platform)
	if artifact.Config.Insecure {
		fmt.Fprintln(out, bundle.InsecureWarning)
	}
	lines := report.New(out)
	for _, image := range artifact.Config.Images {
		lines.Line("Image", fmt.Sprintf("%s %s", image.Ref, image.Digest))
	}
	for _, file := range artifact.Files {
		lines.Line("File", fmt.Sprintf("%s %d bytes", file.Path, len(file.Data)))
	}
	for _, file := range artifact.Evidence {
		lines.Line("Evidence", fmt.Sprintf("%s %d bytes", file.Name, len(file.Data)))
	}
	if into == "" {
		return nil
	}
	return extractEvidence(into, artifact.Evidence)
}

// extractEvidence writes every piece of evidence into dir, or none when one target is already taken.
func extractEvidence(dir string, evidence []bundle.Evidence) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	for _, file := range evidence {
		path := filepath.Join(dir, file.Name)
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("write %s: %w", path, os.ErrExist)
		}
	}
	for _, file := range evidence {
		if err := writeEvidence(filepath.Join(dir, file.Name), file.Data); err != nil {
			return err
		}
	}
	return nil
}

// writeEvidence creates path with data, and removes it again when the write does not finish.
func writeEvidence(path string, data []byte) error {
	handle, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	_, err = handle.Write(data)
	if closeErr := handle.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
