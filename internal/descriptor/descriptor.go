// Package descriptor holds the format-neutral model every reader produces.
package descriptor

// File is one file the bundle carries, with the path it takes under the target root.
type File struct {
	Path string
	Data []byte
}

// Relocation marks the byte range in a file where an image reference lives.
type Relocation struct {
	File   int
	Offset int
	Length int
	Ref    Ref
}

// Manifest is what a reader returns: the files to carry, and where their image references sit.
type Manifest struct {
	Files  []File
	Relocs []Relocation
}
