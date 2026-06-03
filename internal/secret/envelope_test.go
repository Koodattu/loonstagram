package secret

import "testing"

func TestSealAndOpenString(t *testing.T) {
	sealed, err := SealString("admin-token", "https://discord.com/api/webhooks/123/token")
	if err != nil {
		t.Fatalf("SealString() error = %v", err)
	}
	if sealed == "" || sealed == "https://discord.com/api/webhooks/123/token" {
		t.Fatalf("sealed value = %q", sealed)
	}

	opened, err := OpenString("admin-token", sealed)
	if err != nil {
		t.Fatalf("OpenString() error = %v", err)
	}
	if opened != "https://discord.com/api/webhooks/123/token" {
		t.Fatalf("opened = %q", opened)
	}
}

func TestOpenStringAllowsPlaintextMigration(t *testing.T) {
	opened, err := OpenString("admin-token", "plain")
	if err != nil {
		t.Fatalf("OpenString() error = %v", err)
	}
	if opened != "plain" {
		t.Fatalf("opened = %q", opened)
	}
}
