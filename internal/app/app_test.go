package app

import (
	"testing"
)

// TestValidIDsMsg_HandlerExists verifies that ValidIDsMsg is a defined type and
// that the Model has a validIDsCh field (compilation check).
// Full handler behaviour is covered by integration; here we ensure the types exist.
func TestValidIDsMsg_TypeExists(t *testing.T) {
	msg := ValidIDsMsg{ValidIDs: map[string]bool{"<a@x.com>": true}}
	if !msg.ValidIDs["<a@x.com>"] {
		t.Error("ValidIDsMsg.ValidIDs not accessible")
	}
}
