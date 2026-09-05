package attest

import (
	"bytes"
	"errors"
	"fmt"

	"aead.dev/minisign"
)

// keyComment is the first line every minisign key file carries.
var keyComment = []byte("untrusted comment:")

// ErrKeyPassword says the private key is locked and no password came with it.
var ErrKeyPassword = errors.New("the private key is password protected")

// ReadKey opens a minisign private key file, with its password when it carries one.
func ReadKey(text []byte, password string) (minisign.PrivateKey, error) {
	if password != "" {
		key, err := minisign.DecryptKey(password, text)
		if err != nil {
			return minisign.PrivateKey{}, fmt.Errorf("%w: %s", ErrBadKey, err)
		}
		return key, nil
	}
	var key minisign.PrivateKey
	if err := key.UnmarshalText(text); err == nil {
		return key, nil
	}
	if !bytes.HasPrefix(text, keyComment) {
		return minisign.PrivateKey{}, ErrBadKey
	}
	return minisign.PrivateKey{}, fmt.Errorf("%w, pass it in VESSEL_KEY_PASSWORD", ErrKeyPassword)
}
