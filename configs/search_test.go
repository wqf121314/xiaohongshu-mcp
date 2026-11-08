package configs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaxSearchPagesDefault(t *testing.T) {
	assert.Equal(t, DefaultMaxSearchPages, MaxSearchPages())
	assert.Equal(t, 5, DefaultMaxSearchPages)
}

func TestSetMaxSearchPages(t *testing.T) {
	t.Cleanup(func() { _ = SetMaxSearchPages(DefaultMaxSearchPages) })
	assert.NoError(t, SetMaxSearchPages(10))
	assert.Equal(t, 10, MaxSearchPages())
}

func TestSetMaxSearchPagesRejectsNonPositive(t *testing.T) {
	t.Cleanup(func() { _ = SetMaxSearchPages(DefaultMaxSearchPages) })
	for _, n := range []int{0, -3} {
		assert.Error(t, SetMaxSearchPages(n))
		assert.Equal(t, DefaultMaxSearchPages, MaxSearchPages())
	}
}
