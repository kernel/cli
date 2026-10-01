package passwordmanager

import (
	"encoding/base32"
	"net/url"
	"strings"
)

func normalizeTOTP(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "otpauth://") {
		parsed, err := url.Parse(value)
		if err != nil || !strings.EqualFold(parsed.Host, "totp") {
			return ""
		}
		if algorithm := parsed.Query().Get("algorithm"); algorithm != "" && !strings.EqualFold(algorithm, "SHA1") {
			return ""
		}
		if digits := parsed.Query().Get("digits"); digits != "" && digits != "6" {
			return ""
		}
		if period := parsed.Query().Get("period"); period != "" && period != "30" {
			return ""
		}
		value = parsed.Query().Get("secret")
	}
	value = strings.ToUpper(strings.ReplaceAll(value, " ", ""))
	if value == "" {
		return ""
	}
	if _, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.TrimRight(value, "=")); err != nil {
		return ""
	}
	return value
}
