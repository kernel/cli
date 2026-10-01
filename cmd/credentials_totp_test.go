package cmd

import (
	"encoding/json"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/stretchr/testify/require"
)

func TestTotpExtraFields(t *testing.T) {
	digits, period := 8, 60
	extra, err := totpExtraFields("A234567A234567A2", "sha512", &digits, &period)
	require.NoError(t, err)
	params := kernel.CreateCredentialRequestParam{Domain: "example.com", Name: "test", Values: map[string]string{"username": "test"}}
	params.SetExtraFields(extra)
	encoded, err := json.Marshal(params)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(encoded, &body))
	require.Equal(t, "SHA512", body["totp_algorithm"])
	require.Equal(t, float64(8), body["totp_digits"])
	require.Equal(t, float64(60), body["totp_period"])

	for _, test := range []struct {
		secret    string
		algorithm string
		digits    *int
		period    *int
	}{
		{algorithm: "SHA512"},
		{secret: "A234567A234567A2", algorithm: "SHA224"},
		{secret: "A234567A234567A2", digits: new(int)},
		{secret: "A234567A234567A2", period: new(int)},
	} {
		_, err := totpExtraFields(test.secret, test.algorithm, test.digits, test.period)
		require.Error(t, err)
	}
}
