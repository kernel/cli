package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeTotpAlgorithm(t *testing.T) {
	for input, want := range map[string]string{"SHA1": "SHA1", "sha256": "SHA256", " Sha512 ": "SHA512"} {
		got, err := normalizeTotpAlgorithm(input)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	_, err := normalizeTotpAlgorithm("md5")
	assert.Error(t, err)
}
