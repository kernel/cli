package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrDash(t *testing.T) {
	assert.Equal(t, "hello", OrDash("hello"))
	assert.Equal(t, "-", OrDash(""))
}

func TestFirstOrDash(t *testing.T) {
	assert.Equal(t, "first", FirstOrDash("first", "second", "third"))
	assert.Equal(t, "second", FirstOrDash("", "second", "third"))
	assert.Equal(t, "third", FirstOrDash("", "", "third"))
	assert.Equal(t, "-", FirstOrDash("", "", ""))
	assert.Equal(t, "-", FirstOrDash())
}

func TestJoinOrDash(t *testing.T) {
	assert.Equal(t, "a, b, c", JoinOrDash("a", "b", "c"))
	assert.Equal(t, "a", JoinOrDash("a"))
	assert.Equal(t, "-", JoinOrDash())
}
