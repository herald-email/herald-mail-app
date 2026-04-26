package oauth

import (
	"strings"
	"testing"
)

func TestCredentialsPreferEnvironmentOverBuildDefaults(t *testing.T) {
	originalID, originalSecret := defaultClientID, defaultClientSecret
	t.Cleanup(func() {
		defaultClientID = originalID
		defaultClientSecret = originalSecret
	})
	defaultClientID = "build-client-id"
	defaultClientSecret = "build-client-secret"
	t.Setenv("HERALD_GOOGLE_CLIENT_ID", "env-client-id")
	t.Setenv("HERALD_GOOGLE_CLIENT_SECRET", "env-client-secret")

	id, secret, err := credentials()
	if err != nil {
		t.Fatalf("credentials() returned error: %v", err)
	}
	if id != "env-client-id" {
		t.Fatalf("client ID = %q, want env override", id)
	}
	if secret != "env-client-secret" {
		t.Fatalf("client secret = %q, want env override", secret)
	}
}

func TestCredentialsUseBuildDefaultsWhenEnvironmentIsUnset(t *testing.T) {
	originalID, originalSecret := defaultClientID, defaultClientSecret
	t.Cleanup(func() {
		defaultClientID = originalID
		defaultClientSecret = originalSecret
	})
	defaultClientID = "build-client-id"
	defaultClientSecret = "build-client-secret"
	t.Setenv("HERALD_GOOGLE_CLIENT_ID", "")
	t.Setenv("HERALD_GOOGLE_CLIENT_SECRET", "")

	id, secret, err := credentials()
	if err != nil {
		t.Fatalf("credentials() returned error: %v", err)
	}
	if id != "build-client-id" {
		t.Fatalf("client ID = %q, want build default", id)
	}
	if secret != "build-client-secret" {
		t.Fatalf("client secret = %q, want build default", secret)
	}
}

func TestCredentialsErrorWhenNoCredentialSourceIsConfigured(t *testing.T) {
	originalID, originalSecret := defaultClientID, defaultClientSecret
	t.Cleanup(func() {
		defaultClientID = originalID
		defaultClientSecret = originalSecret
	})
	defaultClientID = ""
	defaultClientSecret = ""
	t.Setenv("HERALD_GOOGLE_CLIENT_ID", "")
	t.Setenv("HERALD_GOOGLE_CLIENT_SECRET", "")

	_, _, err := credentials()
	if err == nil {
		t.Fatal("credentials() returned nil error, want missing credential error")
	}
	if !strings.Contains(err.Error(), "HERALD_GOOGLE_CLIENT_ID") {
		t.Fatalf("error = %q, want it to mention missing GitHub/env credential names", err)
	}
}
