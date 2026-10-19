package auth

import (
	"context"
	"testing"

	"spotussi/internal/config"
	"spotussi/internal/db"
)

func TestSetupTokenAnnouncedOncePerProcess(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	var shown []string
	s := New(Options{DB: d, Mode: config.AuthPassword, Hash: fast, AnnounceSetupToken: func(tok string) { shown = append(shown, tok) }})
	for range 3 {
		s.SetupRequired(ctx)
	}
	if len(shown) != 1 || shown[0] != s.SetupToken() {
		t.Fatalf("announced %v", shown)
	}
	if New(Options{DB: d, Mode: config.AuthPassword, AnnounceSetupToken: func(string) {}}).SetupToken() == s.SetupToken() {
		t.Fatal("every process start must get a new token")
	}
	if _, err := s.Setup(ctx, ip, "hunter2hunter2", s.SetupToken()); err != nil {
		t.Fatal(err)
	}
	s.SetupRequired(ctx)
	if len(shown) != 1 {
		t.Fatal("no announcement once setup is done")
	}
	if err := s.ResetPassword(ctx); err != nil {
		t.Fatal(err)
	}
	s.SetupRequired(ctx)
	if len(shown) != 2 {
		t.Fatal("reset-password must re-announce the token")
	}
	// Other modes never announce.
	n := 0
	none := New(Options{DB: d, Mode: config.AuthNone, AnnounceSetupToken: func(string) { n++ }})
	none.SetupRequired(ctx)
	if n != 0 {
		t.Fatal("none mode announced a setup token")
	}
}
