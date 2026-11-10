// Command gendocs writes the CLI reference as Markdown, one page per command.
//
//	go run ./cmd/gendocs [dir]   (default docs/reference)
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
	dir := "docs/reference"
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
	prepend := func(string) string { return "" }
	link := func(name string) string { return strings.ToLower(name) }
	return doc.GenMarkdownTreeCustom(root, dir, prepend, link)
}

func walk(c *cobra.Command, f func(*cobra.Command)) {
	f(c)
	for _, s := range c.Commands() {
		walk(s, f)
	}
}
