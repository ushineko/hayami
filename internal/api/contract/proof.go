package contract

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// ErrBadToken is a token that is not 32 bytes in base64url.
var ErrBadToken = errors.New("the token is not 32 bytes in base64url")

/*
WsProof is the proof that the holder of token is label's side (WsClientLabel
or WsServerLabel), over theirs, the nonce the other side sent, and mine, the
prover's own: HMAC-SHA256 keyed with the token's bytes, base64url without
padding. Binding both nonces makes a proof good for one connection only, and
the label keeps either side's proof from being the other's.
*/
func WsProof(token, label, theirs, mine string) (string, error) {
	key, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(key) != 32 {
		return "", ErrBadToken
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(label + "\n" + theirs + "\n" + mine))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)), nil
}

// WsProofEqual compares a proof received with the one expected, in constant
// time.
func WsProofEqual(got, want string) bool { return hmac.Equal([]byte(got), []byte(want)) }
