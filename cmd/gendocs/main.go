// Command gendocs writes the CLI reference as Markdown, one page per command.
//
//	go run ./cmd/gendocs [dir]   (default docs/src/content/docs/reference)
//
// Pages are written for the Starlight docs site: each starts with frontmatter
// (title, description), cobra's own "## title" heading is dropped because
// Starlight renders the title, and links point at the page routes.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/phuquocchamp/jk/internal/cli"
)

func main() {
	dir := "docs/src/content/docs/reference"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := run(dir); err != nil {
		fmt.Fprintln(os.Stderr, "gendocs:", err)
		os.Exit(1)
	}
}

func run(dir string) error {
	root := cli.NewRootCmd(cli.Deps{Version: "dev"})
	root.InitDefaultCompletionCmd() // cobra adds it lazily at Execute; add it so it is documented
	root.DisableAutoGenTag = true
	walk(root, func(c *cobra.Command) { c.DisableAutoGenTag = true })

	// Remove stale pages so a deleted command does not leave a file behind.
	old, err := filepath.Glob(filepath.Join(dir, "jk*.md"))
	if err != nil {
		return err
	}
	for _, f := range old {
		if err := os.Remove(f); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	short := map[string]string{} // page basename -> short description
	walk(root, func(c *cobra.Command) {
		short[strings.ReplaceAll(c.CommandPath(), " ", "_")+".md"] = c.Short
	})
	prepend := func(file string) string {
		base := filepath.Base(file)
		title := strings.ReplaceAll(strings.TrimSuffix(base, ".md"), "_", " ")
		return fmt.Sprintf("---\ntitle: %q\ndescription: %q\n---\n\n", title, short[base])
	}
	// Absolute routes under the site base (/jk/, see docs/astro.config.mjs).
	link := func(name string) string { return "/jk/reference/" + strings.ToLower(strings.TrimSuffix(name, ".md")) + "/" }
	if err := doc.GenMarkdownTreeCustom(root, dir, prepend, link); err != nil {
		return err
	}
	return fixPages(dir)
}

// fixPages removes the first "## ..." line cobra writes, and the blank line
// after it, since Starlight renders the frontmatter title. It also escapes "<"
// outside code fences so placeholders like <job> are not read as HTML tags.
func fixPages(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "jk*.md"))
	if err != nil {
		return err
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		lines := strings.Split(string(b), "\n")
		inFence := false
		for i, l := range lines {
			if strings.HasPrefix(l, "```") {
				inFence = !inFence
			} else if !inFence {
				lines[i] = strings.ReplaceAll(l, "<", "&lt;")
			}
		}
		for i, l := range lines {
			if strings.HasPrefix(l, "## ") {
				end := i + 1
				if end < len(lines) && lines[end] == "" {
					end++
				}
				lines = append(lines[:i], lines[end:]...)
				break
			}
		}
		if err := os.WriteFile(f, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func walk(c *cobra.Command, f func(*cobra.Command)) {
	f(c)
	for _, s := range c.Commands() {
		walk(s, f)
	}
}
