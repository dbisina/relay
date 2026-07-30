// cmd/relay/providers_signedin_test.go — regression for a real reported bug:
// the UI never reflected whether an account had actually completed sign-in,
// so "Sign in" appeared to do nothing even after a real, successful login.
// accountSignedIn is the fix: a verified, presence-only credential check.

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAccountSignedInUnverifiedProviderIsUnknown(t *testing.T) {
	// A provider with no entry in accountCredentialFile must never be
	// reported as "not signed in": that would be a guess, not a check.
	signedIn, known := accountSignedIn("some-unverified-provider", t.TempDir())
	if known {
		t.Error("unverified provider must report known=false")
	}
	if signedIn {
		t.Error("unverified provider must not report signedIn=true")
	}
}

func TestAccountSignedInClaudeWithCredentials(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	signedIn, known := accountSignedIn("claude", dir)
	if !known {
		t.Fatal("claude is a verified provider, known must be true")
	}
	if !signedIn {
		t.Error("credentials file present, expected signedIn=true")
	}
}

func TestAccountSignedInClaudeWithoutCredentials(t *testing.T) {
	dir := t.TempDir() // empty: exactly the phantom-account scenario
	signedIn, known := accountSignedIn("claude", dir)
	if !known {
		t.Fatal("claude is a verified provider, known must be true")
	}
	if signedIn {
		t.Error("no credentials file present, expected signedIn=false")
	}
}
