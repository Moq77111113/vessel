// Package delivery is the format a user writes: vessel.yaml, and the files, references and relocations a driver reads.
package delivery

import "io/fs"

// File is one file the bundle carries, with the path it takes under the target root.
type File struct {
	Path string
	Data []byte
	Mode fs.FileMode
}

// Relocation marks the byte range in a file where an image reference lives.
type Relocation struct {
	File   int
	Offset int
	Length int
	Ref    Ref
}

// Service is one thing a driver starts, and the file under the target root that declares it.
type Service struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Secret is one secret value a driver puts on the machine.
type Secret struct {
	Name  string
	Value string
}

// Manifest is what a driver reads: files, image references, services, the secrets they read, the directories it fills.
type Manifest struct {
	Files    []File
	Relocs   []Relocation
	Services []Service
	Secrets  []string
	Dirs     []string
}
