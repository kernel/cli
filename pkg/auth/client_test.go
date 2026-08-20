package auth

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRefreshRequiresLoginOnlyForRejectedCredentials(t *testing.T) {
	assert.True(t, refreshRequiresLogin(&TokenRefreshError{StatusCode: http.StatusBadRequest}))
	assert.True(t, refreshRequiresLogin(&TokenRefreshError{StatusCode: http.StatusUnauthorized}))
	assert.False(t, refreshRequiresLogin(&TokenRefreshError{StatusCode: http.StatusInternalServerError}))
	assert.False(t, refreshRequiresLogin(errors.New("network unavailable")))
}
