package main

import "testing"

func TestLoadConfig_RefusesDevLoginWithSecureCookies(t *testing.T) {
	t.Setenv("ARENA_APP_DATABASE_URL", "postgres://x")
	t.Setenv("ARENA_DEV_LOGIN", "true")
	t.Setenv("ARENA_SECURE_COOKIES", "true")
	if _, err := loadConfig(); err == nil {
		t.Fatal("dev login with secure cookies must be refused")
	}
}

func TestLoadConfig_RefusesProviderIDWithoutSecret(t *testing.T) {
	t.Setenv("ARENA_APP_DATABASE_URL", "postgres://x")
	t.Setenv("ARENA_GITHUB_CLIENT_ID", "cid")
	t.Setenv("ARENA_PUBLIC_URL", "https://tolerance.cc")
	if _, err := loadConfig(); err == nil {
		t.Fatal("github id without secret must be refused")
	}
}

func TestLoadConfig_RefusesProviderSecretWithoutID(t *testing.T) {
	t.Setenv("ARENA_APP_DATABASE_URL", "postgres://x")
	t.Setenv("ARENA_GITHUB_CLIENT_SECRET", "sec")
	t.Setenv("ARENA_PUBLIC_URL", "https://tolerance.cc")
	if _, err := loadConfig(); err == nil {
		t.Fatal("github secret without id must be refused")
	}
}

func TestLoadConfig_RefusesProviderKeysWithoutPublicURL(t *testing.T) {
	t.Setenv("ARENA_APP_DATABASE_URL", "postgres://x")
	t.Setenv("ARENA_GITHUB_CLIENT_ID", "cid")
	t.Setenv("ARENA_GITHUB_CLIENT_SECRET", "sec")
	if _, err := loadConfig(); err == nil {
		t.Fatal("github keys without ARENA_PUBLIC_URL must be refused")
	}
}

func TestLoadConfig_ProviderKeysWithPublicURL(t *testing.T) {
	t.Setenv("ARENA_APP_DATABASE_URL", "postgres://x")
	t.Setenv("ARENA_GITHUB_CLIENT_ID", "cid")
	t.Setenv("ARENA_GITHUB_CLIENT_SECRET", "sec")
	t.Setenv("ARENA_PUBLIC_URL", "https://tolerance.cc")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	ps := providersFromConfig(cfg)
	if len(ps) != 1 {
		t.Fatalf("providers: %+v", ps)
	}
	if _, ok := ps["github"]; !ok {
		t.Fatalf("providers: %+v", ps)
	}
}
