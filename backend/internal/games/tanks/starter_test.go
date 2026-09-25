package tanks

import (
	"encoding/json"
	"testing"
)

func TestStarterFiles(t *testing.T) {
	for _, lang := range []string{"python", "js", "javascript"} {
		t.Run(lang, func(t *testing.T) {
			files, err := Starter(lang)
			if err != nil {
				t.Fatalf("Starter(%q) error: %v", lang, err)
			}
			for _, want := range []string{"GAME.md"} {
				if _, ok := files[want]; !ok {
					t.Errorf("Starter(%q) missing %q", lang, want)
				}
			}

			var wantEntry, wantSDK, wantLang string
			switch lang {
			case "python":
				wantEntry, wantSDK, wantLang = "bot.py", "tanks.py", "python"
			default:
				wantEntry, wantSDK, wantLang = "bot.js", "tanks.js", "javascript"
			}
			for _, want := range []string{"bot.json", wantEntry, wantSDK} {
				if _, ok := files[want]; !ok {
					t.Errorf("Starter(%q) missing %q", lang, want)
				}
			}

			botJSON, ok := files["bot.json"]
			if !ok {
				t.Fatalf("Starter(%q) missing bot.json", lang)
			}
			var manifest struct {
				Name     string `json:"name"`
				Language string `json:"language"`
				Entry    string `json:"entry"`
			}
			if err := json.Unmarshal(botJSON, &manifest); err != nil {
				t.Fatalf("bot.json parse error: %v", err)
			}
			if manifest.Language != wantLang {
				t.Errorf("bot.json language = %q, want %q", manifest.Language, wantLang)
			}
			if manifest.Entry != wantEntry {
				t.Errorf("bot.json entry = %q, want %q", manifest.Entry, wantEntry)
			}
		})
	}

	if _, err := Starter("rust"); err == nil {
		t.Errorf("Starter(%q) expected error, got nil", "rust")
	}
}
