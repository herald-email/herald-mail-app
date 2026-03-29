// Package oauth provides Gmail OAuth2 authentication support.
// This is a placeholder package; full implementation is added in subsequent steps.
package oauth

import (
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Scopes required for Gmail IMAP access.
var Scopes = []string{
	"https://mail.google.com/",
}

// NewGmailConfig returns an OAuth2 config for Gmail using the provided
// client ID and secret. The redirect URL should be a localhost callback
// for the desktop auth flow.
func NewGmailConfig(clientID, clientSecret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       Scopes,
		Endpoint:     google.Endpoint,
	}
}
