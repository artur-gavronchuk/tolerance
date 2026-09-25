package tanks

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// starterFS embeds GAME.md, AGENT_TASK.md and every starter kit under
// starter/. Files directly under starter/<lang> must not start with "." or
// "_" — Go's embed directive skips those.
//
//go:embed GAME.md AGENT_TASK.md starter
var starterFS embed.FS

// GameMD is the contents of GAME.md: the tanks rules and protocol,
// shared by the starter kits, the tanks-bot agent task and the site's
// /tanks/docs page.
var GameMD = mustReadStarterFile("GAME.md")

// AgentTaskMD is the contents of AGENT_TASK.md: the TASK.md text handed to
// an agent improving its bot through the tanks-bot proof.
var AgentTaskMD = mustReadStarterFile("AGENT_TASK.md")

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
