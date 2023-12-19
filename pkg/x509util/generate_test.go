package x509util

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerate(t *testing.T) {
	bundleRoot, err := Generate(GenerateOptions{
		Names: []string{"test-root-ca"},
		IsCA:  true,
	})
	require.NoError(t, err)
	crt, _, err := bundleRoot.Decode()
	require.True(t, crt.BasicConstraintsValid)
	require.True(t, crt.IsCA)

	os.WriteFile(filepath.Join("testdata", "root.crt.pem"), bundleRoot.Crt, 0644)
	os.WriteFile(filepath.Join("testdata", "root.key.pem"), bundleRoot.Key, 0644)

	bundleMiddle, err := Generate(GenerateOptions{
		Parent: bundleRoot,
		IsCA:   true,
		Names:  []string{"test-middle-ca"},
	})
	require.NoError(t, err)
	crt, _, err = bundleMiddle.Decode()
	require.True(t, crt.BasicConstraintsValid)
	require.True(t, crt.IsCA)

	os.WriteFile(filepath.Join("testdata", "middle.crt.pem"), bundleMiddle.Crt, 0644)
	os.WriteFile(filepath.Join("testdata", "middle.key.pem"), bundleMiddle.Key, 0644)

	bundleLeaf, err := Generate(GenerateOptions{
		Parent: bundleMiddle,
		Names:  []string{"test-leaf"},
	})
	require.NoError(t, err)
	crt, _, err = bundleLeaf.Decode()
	require.False(t, crt.BasicConstraintsValid)
	require.False(t, crt.IsCA)

	os.WriteFile(filepath.Join("testdata", "leaf.crt.pem"), bundleLeaf.Crt, 0644)
	os.WriteFile(filepath.Join("testdata", "leaf.key.pem"), bundleLeaf.Key, 0644)

}
