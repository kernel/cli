package claude

import (
	_ "embed"
)

// Embedded Playwright scripts for interacting with the Claude extension.
// These scripts are executed via Kernel's Playwright execution API.

//go:embed scripts/send_message.js
var SendMessageScript string

//go:embed scripts/check_status.js
var CheckStatusScript string

//go:embed scripts/stream_chat.js
var StreamChatScript string
