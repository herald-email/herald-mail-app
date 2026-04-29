package app

import "github.com/herald-email/herald-mail-app/internal/ai"

func aiGuidanceNotice(err error) string {
	return ai.MissingModelInstallHint(err)
}
