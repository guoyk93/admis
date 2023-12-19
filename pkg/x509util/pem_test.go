package x509util

import (
	"bytes"
	"crypto/rsa"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeFirstPEM(t *testing.T) {
	buf := &bytes.Buffer{}

	err := pem.Encode(buf, &pem.Block{
		Type:  "AAA",
		Bytes: []byte("aaa"),
	})
	require.NoError(t, err)

	err = pem.Encode(buf, &pem.Block{
		Type:  "BBB",
		Bytes: []byte("bbb"),
	})
	require.NoError(t, err)

	out, err := decodeFirstPEM(buf.Bytes(), "")
	require.NoError(t, err)
	require.Equal(t, []byte("aaa"), out)

	out, err = decodeFirstPEM(buf.Bytes(), "BBB")
	require.NoError(t, err)
	require.Equal(t, []byte("bbb"), out)

	out, err = decodeFirstPEM(buf.Bytes(), "CCC")
	require.Error(t, err)
}

func TestEncodePEM(t *testing.T) {
	buf := encodePEM([]byte("aaa"), "AAA")
	out, err := decodeFirstPEM(buf, "AAA")
	require.NoError(t, err)
	require.Equal(t, []byte("aaa"), out)
}

func TestPEMPair_Decode(t *testing.T) {
	res, err := Generate(GenerateOptions{Names: []string{"aaa"}, IsCA: true})
	require.NoError(t, err)
	crt, key, err := res.Decode()
	require.NoError(t, err)
	require.Equal(t, "aaa", crt.Subject.CommonName)
	require.IsType(t, &rsa.PrivateKey{}, key)
}
