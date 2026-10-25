package release

func Owners(root, name string) (map[string]string, error) {
	deliveries, err := Deliveries(root)
	if err != nil {
		return nil, err
	}
	paths := map[string]string{}
	for _, other := range deliveries {
		if other == name {
			continue
		}
		dir, err := Dir(root, other)
		if err != nil {
			return nil, err
		}
		entry, err := NewRecords(dir).Read()
		if err != nil {
			return nil, err
		}
		for _, file := range entry.Files {
			paths[file.Path] = other
		}
	}
	return paths, nil
}

func Holds(entries []Entry, path string) bool {
	for _, entry := range entries {
		if entry.Path == path {
			return true
		}
	}
	return false
}
