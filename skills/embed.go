// Package skills embeds the agent skills shipped with jk, so 'jk setup skill'
// can install them for users who got the binary with go install.
package skills

import "embed"

// FS holds the jenkins skill directory.
//
//go:embed jenkins
var FS embed.FS
