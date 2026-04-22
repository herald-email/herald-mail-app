package app

import "mail-processor/internal/ai"

func aiGuidanceNotice(err error) string {
	return ai.MissingModelInstallHint(err)
}
