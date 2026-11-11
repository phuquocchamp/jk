package config

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/phuquocchamp/jk/internal/kit/errs"
)

// AddResult says what AddProfile did.
type AddResult struct {
	Path    string // config file path
	Created bool   // the file did not exist and was created
	Added   bool   // the profile was written (false: it already existed)
}

// AddProfile writes profile name to the config file without touching anything
// else in it:
//
//   - no file (or an empty one): it is created with mode 0600 (directory
//     0700), starting with header (each line becomes a "# " comment). When
//     name is not "default", default_profile is set to name so later commands
//     pick it without --profile.
//   - a file without that profile: the profile is appended under "profiles";
//     other profiles, keys and comments are kept, and so is the file mode.
//   - a file that already has the profile: nothing is written (Added false).
//
// The result is loaded back with Load before it replaces the old file, so a
// write never leaves an unreadable config behind.
func AddProfile(o Options, name string, p Profile, header string) (AddResult, error) {
	path, err := Path(o)
	if err != nil {
		return AddResult{}, err
	}
	res := AddResult{Path: path}

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return res, errs.Wrap(errs.CodeInternal, err, "read config "+path)
	}
	existed := err == nil
	mode := fs.FileMode(0o600)
	if existed {
		if st, serr := os.Stat(path); serr == nil {
			mode = st.Mode().Perm()
		}
	}

	var out []byte
	if len(bytes.TrimSpace(data)) == 0 {
		f := File{Profiles: map[string]Profile{name: p}}
		if name != "default" {
			f.DefaultProfile = name
		}
		var doc yaml.Node
		if err := doc.Encode(&f); err != nil {
			return res, errs.Wrap(errs.CodeInternal, err, "encode config")
		}
		annotateTier(&doc)
		body, err := encode(&doc)
		if err != nil {
			return res, err
		}
		out = append(comment(header), body...)
		res.Created = !existed
	} else {
		var doc yaml.Node
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return res, errs.Wrap(errs.CodeUsage, err, "parse config "+path).
				WithHint("fix the YAML in " + path + ", then run the command again")
		}
		root := &doc
		if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
			root = root.Content[0]
		}
		if root.Kind != yaml.MappingNode {
			return res, errs.Usagef("config %s is not a YAML mapping", path)
		}
		profiles := mapValue(root, "profiles")
		if profiles == nil {
			profiles = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			root.Content = append(root.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "profiles"}, profiles)
		}
		if profiles.Kind != yaml.MappingNode {
			return res, errs.Usagef("config %s: profiles is not a mapping", path)
		}
		if mapValue(profiles, name) != nil {
			return res, nil
		}
		var val yaml.Node
		if err := val.Encode(p); err != nil {
			return res, errs.Wrap(errs.CodeInternal, err, "encode profile")
		}
		annotateTier(&val)
		profiles.Style = 0 // block style, even if the file used {} for an empty map
		profiles.Content = append(profiles.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name}, &val)
		if out, err = encode(&doc); err != nil {
			return res, err
		}
	}

	if err := writeAtomic(path, out, mode, "."+o.App+"-*.yaml"); err != nil {
		return res, err
	}
	res.Added = true
	return res, nil
}

// encode writes n as YAML with 2-space indentation.
func encode(n *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, err, "encode config")
	}
	if err := enc.Close(); err != nil {
		return nil, errs.Wrap(errs.CodeInternal, err, "encode config")
	}
	return buf.Bytes(), nil
}

// annotateTier puts the allowed values next to every policy tier under n.
func annotateTier(n *yaml.Node) {
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == "tier" && n.Content[i+1].Kind == yaml.ScalarNode {
				n.Content[i+1].LineComment = "read | write | danger"
			}
		}
	}
	for _, c := range n.Content {
		annotateTier(c)
	}
}

// mapValue returns the value node for key in mapping m, or nil.
func mapValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func comment(header string) []byte {
	if header == "" {
		return nil
	}
	var b bytes.Buffer
	for _, line := range bytes.Split([]byte(header), []byte("\n")) {
		b.WriteString("# ")
		b.Write(line)
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// writeAtomic validates data as a config, then replaces path with it via a
// temp file in the same directory, so readers never see a half-written file.
func writeAtomic(path string, data []byte, mode fs.FileMode, tmpPattern string) error {
	var check File
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&check); err != nil {
		return errs.Wrap(errs.CodeInternal, err, "new config would not load")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errs.Wrap(errs.CodeInternal, err, "create config directory "+dir)
	}
	tmp, err := os.CreateTemp(dir, tmpPattern)
	if err != nil {
		return errs.Wrap(errs.CodeInternal, err, "write config "+path)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return errs.Wrap(errs.CodeInternal, err, "write config "+path)
	}
	if err := tmp.Close(); err != nil {
		return errs.Wrap(errs.CodeInternal, err, "write config "+path)
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return errs.Wrap(errs.CodeInternal, err, "write config "+path)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return errs.Wrap(errs.CodeInternal, err, "write config "+path)
	}
	return nil
}
