package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSSHCompressionArgs(t *testing.T) {
	client := &Client{}

	assert.Equal(t, []string{"-C"}, compressionArgs())
	assert.Contains(t, client.buildSSHArgs(), "-C")
}
