package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAndValidateToken(t *testing.T) {
	secret := "test-secret-32-chars-minimum-ok!"
	tenantID := "tenant-uuid-123"
	email := "user@test.com"

	token, err := GenerateToken(tenantID, email, secret)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	claims, err := ValidateToken(token, secret)
	require.NoError(t, err)
	assert.Equal(t, tenantID, claims.TenantID)
	assert.Equal(t, email, claims.Email)
}

func TestValidateToken_InvalidSecret(t *testing.T) {
	token, _ := GenerateToken("tenant-1", "a@b.com", "correct-secret")
	_, err := ValidateToken(token, "wrong-secret")
	assert.Error(t, err)
}
