package config

import (
	"errors"
	"testing"
)

func sampleProfile() Profile {
	return Profile{Name: "opus-cc", Agent: "claude", Model: "anthropic/claude-opus-4.6"}
}

func TestAddProfile(t *testing.T) {
	cfg := defaults()
	if err := cfg.AddProfile(sampleProfile()); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	got, ok := cfg.Profile("opus-cc")
	if !ok {
		t.Fatal("profile not found after adding")
	}
	if got.Model != "anthropic/claude-opus-4.6" {
		t.Errorf("Model = %q", got.Model)
	}
}

func TestAddProfileRejectsDuplicate(t *testing.T) {
	cfg := defaults()
	if err := cfg.AddProfile(sampleProfile()); err != nil {
		t.Fatalf("first AddProfile: %v", err)
	}
	err := cfg.AddProfile(sampleProfile())
	if !errors.Is(err, ErrProfileExists) {
		t.Errorf("got %v, want ErrProfileExists", err)
	}
}

func TestAddProfileRejectsEmptyFields(t *testing.T) {
	cases := map[string]Profile{
		"no name":  {Agent: "claude", Model: "a/b"},
		"no agent": {Name: "x", Model: "a/b"},
		"no model": {Name: "x", Agent: "claude"},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := defaults()
			if err := cfg.AddProfile(p); !errors.Is(err, ErrProfileInvalid) {
				t.Errorf("got %v, want ErrProfileInvalid", err)
			}
		})
	}
}

func TestProfileLookupIsCaseInsensitive(t *testing.T) {
	cfg := defaults()
	if err := cfg.AddProfile(sampleProfile()); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if _, ok := cfg.Profile("OPUS-CC"); !ok {
		t.Error("lookup should be case-insensitive")
	}
}

func TestRemoveProfile(t *testing.T) {
	cfg := defaults()
	if err := cfg.AddProfile(sampleProfile()); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if err := cfg.RemoveProfile("opus-cc"); err != nil {
		t.Fatalf("RemoveProfile: %v", err)
	}
	if _, ok := cfg.Profile("opus-cc"); ok {
		t.Error("profile still present after removal")
	}
}

func TestRemoveProfileMissing(t *testing.T) {
	cfg := defaults()
	if err := cfg.RemoveProfile("nope"); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("got %v, want ErrProfileNotFound", err)
	}
}

func TestRenameProfile(t *testing.T) {
	cfg := defaults()
	if err := cfg.AddProfile(sampleProfile()); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if err := cfg.RenameProfile("opus-cc", "flagship"); err != nil {
		t.Fatalf("RenameProfile: %v", err)
	}
	if _, ok := cfg.Profile("opus-cc"); ok {
		t.Error("old name still resolves")
	}
	got, ok := cfg.Profile("flagship")
	if !ok {
		t.Fatal("new name does not resolve")
	}
	if got.Model != "anthropic/claude-opus-4.6" {
		t.Errorf("Model = %q, want it carried over", got.Model)
	}
}

func TestRenameProfileRejectsExistingTarget(t *testing.T) {
	cfg := defaults()
	if err := cfg.AddProfile(sampleProfile()); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if err := cfg.AddProfile(Profile{Name: "other", Agent: "claude", Model: "a/b"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if err := cfg.RenameProfile("opus-cc", "other"); !errors.Is(err, ErrProfileExists) {
		t.Errorf("got %v, want ErrProfileExists", err)
	}
}

func TestRemoveProfilePreservesOrder(t *testing.T) {
	cfg := defaults()
	for _, name := range []string{"c", "a", "b"} {
		if err := cfg.AddProfile(Profile{Name: name, Agent: "claude", Model: "x/y"}); err != nil {
			t.Fatalf("AddProfile %s: %v", name, err)
		}
	}
	if err := cfg.RemoveProfile("a"); err != nil {
		t.Fatalf("RemoveProfile: %v", err)
	}
	if len(cfg.Profiles) != 2 || cfg.Profiles[0].Name != "c" || cfg.Profiles[1].Name != "b" {
		t.Errorf("order not preserved: %+v", cfg.Profiles)
	}
}

func TestAddProfileRejectsDuplicateCaseInsensitive(t *testing.T) {
	cfg := defaults()
	if err := cfg.AddProfile(Profile{Name: "opus-cc", Agent: "claude", Model: "anthropic/claude-opus-4.6"}); err != nil {
		t.Fatalf("first AddProfile: %v", err)
	}
	err := cfg.AddProfile(Profile{Name: "OPUS-CC", Agent: "claude", Model: "anthropic/claude-opus-4.6"})
	if !errors.Is(err, ErrProfileExists) {
		t.Errorf("got %v, want ErrProfileExists", err)
	}
}

func TestRenameProfileAllowsSelfWithCaseChange(t *testing.T) {
	cfg := defaults()
	if err := cfg.AddProfile(sampleProfile()); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if err := cfg.RenameProfile("opus-cc", "OPUS-CC"); err != nil {
		t.Fatalf("RenameProfile: %v", err)
	}
	got, ok := cfg.Profile("OPUS-CC")
	if !ok {
		t.Fatal("new name does not resolve")
	}
	if got.Name != "OPUS-CC" {
		t.Errorf("stored name = %q, want OPUS-CC", got.Name)
	}
}

func TestAddProfileTrimsWhitespace(t *testing.T) {
	cfg := defaults()
	if err := cfg.AddProfile(Profile{Name: "  padded  ", Agent: "claude", Model: "a/b"}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if _, ok := cfg.Profile("padded"); !ok {
		t.Error("trimmed profile not found")
	}
}
