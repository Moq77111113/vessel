package delivery

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// DeliveryFile is the file a source directory declares its delivery in.
const DeliveryFile = "vessel.yaml"

// Errors Read returns.
var (
	ErrNoDelivery        = errors.New("no delivery declaration")
	ErrNoName            = errors.New("the delivery carries no name")
	ErrDeliveryName      = errors.New("the delivery name is not a plain lowercase identifier")
	ErrVariableName      = errors.New("a variable name is not upper snake case")
	ErrTargetNotAbsolute = errors.New("a file target is not an absolute path")
	ErrTargetEscapes     = errors.New("a file target leaves the target root")
	ErrTargetNotClean    = errors.New("a file target is not a clean path")
	ErrTargetIsRoot      = errors.New("a file target names no file")
	ErrFileSource        = errors.New("a file carries no source")
	ErrSourceEscapes     = errors.New("a source leaves the delivery directory")
	ErrFileMode          = errors.New("a file mode is not octal permission bits, write it as 0600")
)

var variableName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

var deliveryName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Mapping is one plain file the delivery carries, and where it lands on the machine.
type Mapping struct {
	Source string `yaml:"source" json:"source"`
	Target string `yaml:"target" json:"target"`
	Mode   Mode   `yaml:"mode,omitempty" json:"mode,omitempty"`
}

// Mode is a file's permission bits, written in octal: 0600.
type Mode fs.FileMode

// UnmarshalYAML reads the octal digits as they are written, refusing anything but permission bits.
func (m *Mode) UnmarshalYAML(node *yaml.Node) error {
	bits, err := strconv.ParseUint(node.Value, 8, 32)
	if err != nil || bits == 0 || bits > 0o777 {
		return fmt.Errorf("mode %q: %w", node.Value, ErrFileMode)
	}
	*m = Mode(bits)
	return nil
}

// Variable is a site value the machine supplies at install time.
type Variable struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	From        string `yaml:"from,omitempty" json:"from,omitempty"`
	Secret      bool   `yaml:"secret,omitempty" json:"secret,omitempty"`
}

// Delivery is what vessel.yaml declares.
type Delivery struct {
	Name      string     `yaml:"name"`
	Version   string     `yaml:"version"`
	Source    string     `yaml:"source"`
	Files     []Mapping  `yaml:"files"`
	Variables []Variable `yaml:"variables"`
	Actions   []string   `yaml:"actions"`
}

// CheckName refuses a name that cannot stand for a directory under the target root.
func CheckName(name string) error {
	if name == "" {
		return fmt.Errorf("%s: %w", DeliveryFile, ErrNoName)
	}
	if !deliveryName.MatchString(name) {
		return fmt.Errorf("%q: %w", name, ErrDeliveryName)
	}
	return nil
}

// SecretNames names every secret a delivery declares.
func SecretNames(variables []Variable) []string {
	var names []string
	for _, variable := range variables {
		if variable.Secret {
			names = append(names, variable.Name)
		}
	}
	return names
}

func leaves(target string) bool {
	for _, segment := range strings.Split(target, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

// Read returns the declaration the directory carries, and ErrNoDelivery when it carries none.
func Read(dir fs.FS) (Delivery, error) {
	data, err := fs.ReadFile(dir, DeliveryFile)
	if errors.Is(err, fs.ErrNotExist) {
		return Delivery{}, fmt.Errorf("%s: %w", DeliveryFile, ErrNoDelivery)
	}
	if err != nil {
		return Delivery{}, fmt.Errorf("read %s: %w", DeliveryFile, err)
	}
	var declaration Delivery
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&declaration); err != nil && !errors.Is(err, io.EOF) {
		return Delivery{}, fmt.Errorf("read %s: %w", DeliveryFile, err)
	}
	if err := CheckName(declaration.Name); err != nil {
		return Delivery{}, err
	}
	source, err := local(declaration.Source)
	if err != nil {
		return Delivery{}, err
	}
	declaration.Source = source
	for _, variable := range declaration.Variables {
		if !variableName.MatchString(variable.Name) {
			return Delivery{}, fmt.Errorf("%q: %w", variable.Name, ErrVariableName)
		}
	}
	for i, file := range declaration.Files {
		if file.Source == "" {
			return Delivery{}, fmt.Errorf("target %q: %w", file.Target, ErrFileSource)
		}
		source, err := local(file.Source)
		if err != nil {
			return Delivery{}, err
		}
		declaration.Files[i].Source = source
		if len(file.Target) == 0 || file.Target[0] != '/' {
			return Delivery{}, fmt.Errorf("%q: %w", file.Target, ErrTargetNotAbsolute)
		}
		if leaves(file.Target) {
			return Delivery{}, fmt.Errorf("%q: %w", file.Target, ErrTargetEscapes)
		}
		if path.Clean(file.Target) != file.Target {
			return Delivery{}, fmt.Errorf("%q: %w", file.Target, ErrTargetNotClean)
		}
		if file.Target == "/" {
			return Delivery{}, fmt.Errorf("%q: %w", file.Target, ErrTargetIsRoot)
		}
	}
	return declaration, nil
}

func local(name string) (string, error) {
	clean := path.Clean(name)
	if !fs.ValidPath(clean) {
		return "", fmt.Errorf("%q: %w", name, ErrSourceEscapes)
	}
	return clean, nil
}
