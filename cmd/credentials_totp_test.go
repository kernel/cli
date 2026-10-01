package cmd

import (
	"encoding/json"
	"testing"

	"github.com/kernel/kernel-go-sdk"
	"github.com/stretchr/testify/require"
)

func TestTypedTotpFields(t *testing.T) {
	digits, period := 8, 60
	require.NoError(t, validateTotpSettings("A234567A234567A2", "sha512", &digits, &period))
	params := kernel.CreateCredentialRequestParam{
		Domain:        "example.com",
		Name:          "test",
		Values:        map[string]string{"username": "test"},
		TotpSecret:    kernel.String("A234567A234567A2"),
		TotpAlgorithm: kernel.CreateCredentialRequestTotpAlgorithmSha512,
		TotpDigits:    kernel.Int(8),
		TotpPeriod:    kernel.Int(60),
	}
	encoded, err := json.Marshal(params)
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, json.Unmarshal(encoded, &body))
	require.Equal(t, "SHA512", body["totp_algorithm"])
	require.Equal(t, float64(8), body["totp_digits"])
	require.Equal(t, float64(60), body["totp_period"])

	update := kernel.UpdateCredentialRequestParam{
		TotpSecret:    kernel.String("A234567A234567A2"),
		TotpAlgorithm: kernel.UpdateCredentialRequestTotpAlgorithmSha512,
		TotpDigits:    kernel.Int(8),
		TotpPeriod:    kernel.Int(60),
	}
	encoded, err = json.Marshal(update)
	require.NoError(t, err)
	body = make(map[string]any)
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
		require.Error(t, validateTotpSettings(test.secret, test.algorithm, test.digits, test.period))
	}
}
