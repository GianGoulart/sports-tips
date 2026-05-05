package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKelly_PositiveEdge(t *testing.T) {
	result := Kelly(2.30, 0.55)

	assert.InDelta(t, 0.115, result.Edge, 0.001)
	assert.InDelta(t, 0.204, result.KellyFull, 0.001)
	assert.InDelta(t, 0.102, result.KellyHalf, 0.001)
	assert.True(t, result.IsValueBet)
}

func TestKelly_NegativeEdge(t *testing.T) {
	result := Kelly(1.50, 0.50)

	assert.Less(t, result.Edge, 0.0)
	assert.False(t, result.IsValueBet)
	assert.Equal(t, 0.0, result.KellyFull)
	assert.Equal(t, 0.0, result.KellyHalf)
}
