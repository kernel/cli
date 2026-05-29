package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserErrorHelpers(t *testing.T) {
	assert.Equal(t, "--name is required; add --name <name>", RequiredFlag("--name", "<name>").Error())
	assert.Equal(t, "missing browser ID; use: kernel browsers get <id>", RequiredArg("browser ID", "kernel browsers get <id>").Error())
	assert.Equal(t, "choose only one of --profile-id or --profile-name", ChooseOnlyOne("--profile-id", "--profile-name").Error())
	assert.Equal(t, "set at least one of --proxy-id, --profile-id, or --viewport", SetAtLeastOne("--proxy-id", "--profile-id", "--viewport").Error())
	assert.Equal(t, "invalid --status \"bad\"; use one of: active, deleted, all", InvalidChoice("--status", "bad", "active", "deleted", "all").Error())
	assert.Equal(t, "Browser \"brw_123\" not found; run `kernel browsers list` to find valid IDs", NotFound("Browser", "brw_123", "kernel browsers list").Error())
}
