package quadlet

import "bytes"

// cutKey splits a key from its value on a systemd unit line already stripped of comments and
// leading whitespace. systemd accepts whitespace on both sides of the equals, so Secret=x,
// Secret = x and Secret =x are all the same key.
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
