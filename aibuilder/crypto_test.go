package aibuilder

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func randomKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	e, err := NewEncryptor(randomKey(t))
	require.NoError(t, err)

	plaintext := "sk-ant-api03-verysecretkey"
	ct, err := e.Encrypt(plaintext)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, ct)

	pt, err := e.Decrypt(ct)
	require.NoError(t, err)
	assert.Equal(t, plaintext, pt)
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	e, err := NewEncryptor(randomKey(t))
	require.NoError(t, err)

	a, err := e.Encrypt("same plaintext")
	require.NoError(t, err)
	b, err := e.Encrypt("same plaintext")
	require.NoError(t, err)

	assert.NotEqual(t, a, b, "nonce randomization should produce different ciphertexts")
}

func TestInvalidKeyRejected(t *testing.T) {
	_, err := NewEncryptor([]byte("too short"))
	assert.ErrorIs(t, err, ErrInvalidKey)
}

func TestDecryptTamperedFails(t *testing.T) {
	e, err := NewEncryptor(randomKey(t))
	require.NoError(t, err)

	ct, err := e.Encrypt("secret")
	require.NoError(t, err)

	tampered := ct[:len(ct)-2] + "XX"
	_, err = e.Decrypt(tampered)
	assert.Error(t, err)
}

func TestMask(t *testing.T) {
	assert.Equal(t, "…", Mask("ab"))
	assert.Equal(t, "…", Mask("abcd"))
	assert.Equal(t, "…cdef", Mask("abcdef"))
	assert.Equal(t, "…yz12", Mask("sk-ant-abcxyz12"))
}
