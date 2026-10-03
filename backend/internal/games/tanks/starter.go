package tanks

import (
	"archive/zip"
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// AgentPrompt is the prompt the starter kit's README and the "My bot" page hand to the owner's own coding
// agent. The frontend shows the same text; keep them in step.
const AgentPrompt = `Read GAME.md in this folder, then improve the tank bot (bot.py or bot.js, whichever is here) so it beats the house bots hunter and sniper. Keep bot.json valid and keep the same entry file. Use only the standard library of the language. You do not need to install or run anything: the platform plays the bot for you once the folder is zipped and uploaded.`

// StarterReadme is the README.md of the downloadable starter kit.
func StarterReadme() string {
	return `# Tanks starter kit

A working tank bot for tolerance. It already passes the platform checks; your job is to make it win.

1. Give this whole folder to your coding agent (Claude Code, Cursor, Codex, anything) with this prompt:

    ` + strings.ReplaceAll(AgentPrompt, "\n", "\n    ") + `

2. Zip the folder (the folder itself or its contents, both work) and upload the .zip on the My bot page.
3. The platform plays a trial match and then ladder matches for you. Open the replays on the site, paste
   what went wrong back to your agent, and upload the next version.

Files: GAME.md (rules and protocol), bot.json (name, language, entry), the bot and the tanks SDK module.
Optional, advanced: the arena CLI can run matches locally (arena tanks play . house:hunter).
`
}

// StarterZip returns lang's starter kit as a zip archive: the files of Starter plus README.md, all under a
// single top-level folder ("tanks-starter-python/"). It is built from the same embedded files as
// `arena tanks new`, so the two cannot drift. The second result is the suggested file name.
func StarterZip(lang string) ([]byte, string, error) {
	files, err := Starter(lang)
	if err != nil {
		return nil, "", err
	}
	files["README.md"] = []byte(StarterReadme())
	dir, _ := starterLangDir(lang)
	root := "tanks-starter-" + dir

	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.Create(root + "/" + n)
		if err != nil {
			return nil, "", err
		}
		if _, err := w.Write(files[n]); err != nil {
			return nil, "", err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), root + ".zip", nil
}

// starterFS embeds GAME.md and every starter kit under
// starter/. Files directly under starter/<lang> must not start with "." or
// "_" — Go's embed directive skips those.
//
//go:embed GAME.md starter
var starterFS embed.FS

// GameMD is the contents of GAME.md: the tanks rules and protocol,
// shared by the starter kits and the site's
// /tanks/docs page.
var GameMD = mustReadStarterFile("GAME.md")

func mustReadStarterFile(name string) string {
	data, err := starterFS.ReadFile(name)
	if err != nil {
		panic(fmt.Sprintf("tanks: embedded %s missing: %v", name, err))
	}
	return string(data)
}

// starterLangDir maps a starter kit language, in every spelling the
// platform accepts, to its directory under starter/.
func starterLangDir(lang string) (string, bool) {
	switch strings.ToLower(lang) {
	case "python":
		return "python", true
	case "js", "javascript":
		return "js", true
	default:
		return "", false
	}
}

// Starter returns lang's starter kit ("python", "js" or "javascript") as a
// map of file path, relative to the bot's root directory, to contents. It
// always includes "GAME.md" alongside the kit's own files (bot.json, the
// entry point and the tanks SDK module).
func Starter(lang string) (map[string][]byte, error) {
	dir, ok := starterLangDir(lang)
	if !ok {
		return nil, fmt.Errorf("tanks: unknown starter language %q", lang)
	}

	root := "starter/" + dir
	files := map[string][]byte{}
	err := fs.WalkDir(starterFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := starterFS.ReadFile(path)
		if err != nil {
			return err
		}
		files[strings.TrimPrefix(path, root+"/")] = data
		return nil
	})
	if err != nil {
		return nil, err
	}

	files["GAME.md"] = []byte(GameMD)
	return files, nil
}
