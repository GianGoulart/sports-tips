package results

import (
	"testing"
	"github.com/stretchr/testify/assert"
)

func TestDeriveOutcome(t *testing.T) {
	assert.Equal(t, "home", DeriveOutcome(2, 1))
	assert.Equal(t, "away", DeriveOutcome(0, 3))
	assert.Equal(t, "draw", DeriveOutcome(1, 1))
	assert.Equal(t, "draw", DeriveOutcome(0, 0))
}

func TestFuzzyMatchTeam(t *testing.T) {
	assert.True(t, fuzzyMatchTeam("Manchester United", "Man United"))
	assert.True(t, fuzzyMatchTeam("Arsenal FC", "Arsenal"))
	assert.True(t, fuzzyMatchTeam("Real Madrid", "Real Madrid"))
	assert.False(t, fuzzyMatchTeam("Arsenal", "Chelsea"))
}
