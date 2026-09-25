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
