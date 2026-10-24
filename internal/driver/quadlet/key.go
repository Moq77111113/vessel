package quadlet

import "bytes"

func cutKey(line []byte, key string) ([]byte, bool) {
	rest, ok := bytes.CutPrefix(line, []byte(key))
	if !ok {
		return nil, false
	}
	rest = bytes.TrimLeft(rest, " \t")
	rest, ok = bytes.CutPrefix(rest, []byte("="))
	if !ok {
		return nil, false
	}
	return bytes.TrimLeft(rest, " \t"), true
}
