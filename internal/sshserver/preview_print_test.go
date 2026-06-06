package sshserver

import (
	"testing"

	"github.com/herald-email/herald-mail-app/internal/app"
)

func TestSSHSessionModelForcesPreviewPrintingUnsupported(t *testing.T) {
	model, _ := newSessionModel(nil, sshServerOptions{DemoMode: true})
	m, ok := model.(*app.Model)
	if !ok {
		t.Fatalf("newSessionModel returned %T, want *app.Model", model)
	}
	if !m.PreviewPrintingUnsupportedForTest() {
		t.Fatal("SSH session should force unsupported preview printer")
	}
}
