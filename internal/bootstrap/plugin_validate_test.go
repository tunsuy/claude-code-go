package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `plugin validate` tests, byte-pinned against the oracle captures in
// /tmp/ccg/valfix/cap* (claude v2.1.261; the probe argv is noted per subtest).
// The probe fixtures are rebuilt under t.TempDir() and their paths
// interpolated into the expected texts.
//
// Documented divergence: the JSON parse-error detail is Go's wording
// ("invalid character 'o' in literal null (expecting 'u')"); the oracle's JS
// parser says `Unexpected identifier "no"`.  The surrounding format matches.

// Long finding messages shared by several expected texts.
const (
	pluginVFrontmatter = "No frontmatter block found. Add YAML frontmatter between --- " +
		"delimiters at the top of the file to set description and other metadata."
	pluginVNoVersion     = `No version specified. Consider adding a version following semver (e.g., "1.0.0")`
	pluginVNoDescription = "No description provided. Adding a description helps users understand what your plugin does"
	pluginVNoAuthor      = "No author information provided. Consider adding author details for plugin attribution"
	pluginVNoMPDesc      = "No marketplace description provided. Adding a description helps users " +
		"understand what this marketplace offers"
	pluginVHooksRoot = "hooks.json must have `hooks` (the hook matchers) or `modules` " +
		"(hooks modules), or both"
)

// pluginVMismatch renders the oracle's entry-vs-plugin.json version warning.
func pluginVMismatch(entry, rel, version string) string {
	return fmt.Sprintf(`Entry declares version %q but %s says %q. At install time, `+
		`plugin.json wins (calculatePluginVersion precedence) — the entry version is `+
		`silently ignored. Update this entry to %q to match.`, entry, rel, version, version)
}

// writeValidateFiles writes rel→content under base and returns base.
func writeValidateFiles(t *testing.T, base string, files map[string]string) string {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(base, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return base
}

// expectValidate runs `plugin validate <argv…>` against fresh deps and
// byte-compares stdout.  stderr must stay empty (the oracle never uses it
// here); the error must be nil (exit 0) or ErrSilent (exit 1).
func expectValidate(t *testing.T, want string, wantFail bool, argv ...string) {
	t.Helper()
	out, errb, _ := swapPluginDeps(t)
	err := runPlugin(t, out, errb, append([]string{"plugin", "validate"}, argv...)...)
	if wantFail {
		if !errors.Is(err, ErrSilent) {
			t.Fatalf("err = %v, want ErrSilent", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	if out.String() != want {
		t.Fatalf("stdout:\n got %q\nwant %q", out.String(), want)
	}
	if errb.Len() != 0 {
		t.Fatalf("stderr = %q", errb.String())
	}
}

func TestPluginValidatePluginManifest(t *testing.T) {
	t.Run("perfect-strict", func(t *testing.T) { // probe D
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/plugin.json": `{"name":"perfect","version":"1.0.0","description":"P","author":{"name":"p"}}`,
		})
		pj := filepath.Join(base, ".claude-plugin", "plugin.json")
		expectValidate(t, "Validating plugin manifest: "+pj+"\n\n✔ Validation passed\n", false, base, "--strict")
	})
	t.Run("badplugin-strict", func(t *testing.T) { // probe C
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/plugin.json": `{"name":"badplugin"}`,
		})
		pj := filepath.Join(base, ".claude-plugin", "plugin.json")
		want := fmt.Sprintf("Validating plugin manifest: %s\n\n"+
			"⚠ Found 3 warnings:\n\n"+
			"  ❯ version: %s\n"+
			"  ❯ description: %s\n"+
			"  ❯ author: %s\n\n"+
			"✘ Validation failed (--strict treats warnings as errors)\n",
			pj, pluginVNoVersion, pluginVNoDescription, pluginVNoAuthor)
		expectValidate(t, want, true, base, "--strict")
	})
	t.Run("badplugin-json-strict", func(t *testing.T) { // probe Z
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/plugin.json": `{"name":"badplugin"}`,
		})
		pj := filepath.Join(base, ".claude-plugin", "plugin.json")
		want := fmt.Sprintf(`{
  "success": false,
  "strict": true,
  "target": %q,
  "manifest": {
    "file": %q,
    "type": "plugin",
    "errors": [],
    "warnings": [
      {
        "path": "version",
        "message": %q,
        "code": null
      },
      {
        "path": "description",
        "message": %q,
        "code": null
      },
      {
        "path": "author",
        "message": %q,
        "code": null
      }
    ],
    "notes": []
  },
  "contents": []
}
`, pj, pj, pluginVNoVersion, pluginVNoDescription, pluginVNoAuthor)
		expectValidate(t, want, true, base, "--json", "--strict")
	})
	t.Run("extra-positional-ignored", func(t *testing.T) { // probe AA
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/plugin.json": `{"name":"perfect","version":"1.0.0","description":"P","author":{"name":"p"}}`,
		})
		pj := filepath.Join(base, ".claude-plugin", "plugin.json")
		expectValidate(t, "Validating plugin manifest: "+pj+"\n\n✔ Validation passed\n", false, base, "extra-arg")
	})
	t.Run("loose-manifest-file-skips-contents", func(t *testing.T) { // probe Y
		// A bare JSON file target validates alone: the sibling commands/
		// directory is not scanned (no .claude-plugin/plugin.json layout).
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			"bare.json":       `{"name":"b","version":"1.0.0","description":"B","author":{"name":"p"}}`,
			"commands/bad.md": "no fm here\n",
		})
		bare := filepath.Join(base, "bare.json")
		expectValidate(t, "Validating plugin manifest: "+bare+"\n\n✔ Validation passed\n", false, bare)
	})
}

func TestPluginValidateFileTargets(t *testing.T) {
	t.Run("markdown-file-target", func(t *testing.T) { // probe F
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			"no-fm.md": "no fm\n",
		})
		md := filepath.Join(base, "no-fm.md")
		want := fmt.Sprintf("Validating plugin manifest: %s\n\n"+
			"✘ Found 1 error:\n\n"+
			"  ❯ json: Invalid JSON syntax: JSON Parse error: invalid character 'o' in literal null (expecting 'u')\n\n"+
			"✘ Validation failed\n", md)
		expectValidate(t, want, true, md)
	})
	t.Run("hooks-file-target", func(t *testing.T) { // probe G
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			"hooks.json": "{}",
		})
		hj := filepath.Join(base, "hooks.json")
		want := "Validating plugin manifest: " + hj + "\n\n" +
			"✘ Found 1 error:\n\n  ❯ name: Invalid input\n\n✘ Validation failed\n"
		expectValidate(t, want, true, hj)
	})
	t.Run("notjson-json", func(t *testing.T) { // probe O
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			"notjson.txt": "not-json\n",
		})
		txt := filepath.Join(base, "notjson.txt")
		want := fmt.Sprintf(`{
  "success": false,
  "strict": false,
  "target": %q,
  "manifest": {
    "file": %q,
    "type": "plugin",
    "errors": [
      {
        "path": "json",
        "message": "Invalid JSON syntax: JSON Parse error: invalid character 'o' in literal null (expecting 'u')",
        "code": null
      }
    ],
    "warnings": [],
    "notes": []
  },
  "contents": []
}
`, txt, txt)
		expectValidate(t, want, true, txt, "--json")
	})
}

// fullFixture is the /tmp/ccg/full probe shape: a complete manifest plus one
// frontmatter-less command and an empty-object hooks file.
func fullFixture(t *testing.T) (base, pj, cmd, hooks string) {
	t.Helper()
	base = writeValidateFiles(t, t.TempDir(), map[string]string{
		".claude-plugin/plugin.json": `{"name":"full","version":"1.0.0","description":"Full","author":{"name":"p","email":"p@x.test"}}`,
		"commands/no-fm.md":          "no fm\n",
		"hooks/hooks.json":           "{}",
	})
	return base, filepath.Join(base, ".claude-plugin", "plugin.json"),
		filepath.Join(base, "commands", "no-fm.md"), filepath.Join(base, "hooks", "hooks.json")
}

func TestPluginValidateFullContents(t *testing.T) {
	wantText := func(pj, cmd, hooks string) string {
		return fmt.Sprintf("Validating plugin manifest: %s\n\n"+
			"Validating command: %s\n\n"+
			"⚠ Found 1 warning:\n\n"+
			"  ❯ frontmatter: %s\n\n"+
			"Validating hooks: %s\n\n"+
			"✘ Found 1 error:\n\n"+
			"  ❯ root: %s\n\n"+
			"✘ Validation failed\n", pj, cmd, pluginVFrontmatter, hooks, pluginVHooksRoot)
	}
	t.Run("manifest-file-target", func(t *testing.T) { // probe E
		_, pj, cmd, hooks := fullFixture(t)
		expectValidate(t, wantText(pj, cmd, hooks), true, pj)
	})
	t.Run("dir-target-strict", func(t *testing.T) { // probe J — same bytes as E
		base, pj, cmd, hooks := fullFixture(t)
		expectValidate(t, wantText(pj, cmd, hooks), true, base, "--strict")
	})
	t.Run("dir-target-json-strict", func(t *testing.T) { // probe K
		base, pj, cmd, hooks := fullFixture(t)
		want := fmt.Sprintf(`{
  "success": false,
  "strict": true,
  "target": %q,
  "manifest": {
    "file": %q,
    "type": "plugin",
    "errors": [],
    "warnings": [],
    "notes": []
  },
  "contents": [
    {
      "file": %q,
      "type": "command",
      "errors": [],
      "warnings": [
        {
          "path": "frontmatter",
          "message": %q,
          "code": null
        }
      ],
      "notes": []
    },
    {
      "file": %q,
      "type": "hooks",
      "errors": [
        {
          "path": "root",
          "message": %q,
          "code": null
        }
      ],
      "warnings": [],
      "notes": []
    }
  ]
}
`, pj, pj, cmd, pluginVFrontmatter, hooks, pluginVHooksRoot)
		expectValidate(t, want, true, base, "--json", "--strict")
	})
}

// full2Fixture is the /tmp/ccg/valfix/full2 probe shape: findings in every
// content group plus loose markdown that must not be scanned.
func full2Fixture(t *testing.T) (base string, paths map[string]string) {
	t.Helper()
	base = writeValidateFiles(t, t.TempDir(), map[string]string{
		".claude-plugin/plugin.json": `{"name":"full2","version":"1.0.0","description":"F","author":{"name":"p"}}`,
		"skills/nofm/SKILL.md":       "skill no fm\n",
		"agents/noagent.md":          "agent no fm\n",
		"agents/okagent.md":          "---\nname: ok\n---\nbody\n",
		"commands/bad.md":            "no frontmatter\n",
		"commands/sub/deep.md":       "deep no fm\n",
		"hooks/hooks.json":           "{}",
		"other.md":                   "loose md\n",
		"README.md":                  "readme body\n",
	})
	paths = map[string]string{}
	for _, rel := range []string{
		"skills/nofm/SKILL.md", "agents/noagent.md", "agents/okagent.md",
		"commands/bad.md", "commands/sub/deep.md", "hooks/hooks.json",
	} {
		paths[rel] = filepath.Join(base, filepath.FromSlash(rel))
	}
	return base, paths
}

func TestPluginValidateFull2Text(t *testing.T) { // probe L
	base, p := full2Fixture(t)
	want := fmt.Sprintf("Validating plugin manifest: %s\n\n"+
		"Validating skill: %s\n\n⚠ Found 1 warning:\n\n  ❯ frontmatter: %s\n\n"+
		"Validating agent: %s\n\n⚠ Found 1 warning:\n\n  ❯ frontmatter: %s\n\n"+
		"Validating agent: %s\n\n⚠ Found 1 warning:\n\n  ❯ description: No description in frontmatter. A description helps users and Claude understand when to use this agent.\n\n"+
		"Validating command: %s\n\n⚠ Found 1 warning:\n\n  ❯ frontmatter: %s\n\n"+
		"Validating command: %s\n\n⚠ Found 1 warning:\n\n  ❯ frontmatter: %s\n\n"+
		"Validating hooks: %s\n\n✘ Found 1 error:\n\n  ❯ root: %s\n\n"+
		"✘ Validation failed\n",
		filepath.Join(base, ".claude-plugin", "plugin.json"),
		p["skills/nofm/SKILL.md"], pluginVFrontmatter,
		p["agents/noagent.md"], pluginVFrontmatter,
		p["agents/okagent.md"],
		p["commands/bad.md"], pluginVFrontmatter,
		p["commands/sub/deep.md"], pluginVFrontmatter,
		p["hooks/hooks.json"], pluginVHooksRoot)
	expectValidate(t, want, true, base)
}

func TestPluginValidateFull2JSON(t *testing.T) { // probe L2
	base, p := full2Fixture(t)
	section := func(file, typ, key, msg string) string {
		return fmt.Sprintf(`    {
      "file": %q,
      "type": %q,
      "errors": [],
      "warnings": [
        {
          "path": %q,
          "message": %q,
          "code": null
        }
      ],
      "notes": []
    },`, file, typ, key, msg)
	}
	contents := []string{
		section(p["skills/nofm/SKILL.md"], "skill", "frontmatter", pluginVFrontmatter),
		section(p["agents/noagent.md"], "agent", "frontmatter", pluginVFrontmatter),
		section(p["agents/okagent.md"], "agent", "description",
			"No description in frontmatter. A description helps users and Claude understand when to use this agent."),
		section(p["commands/bad.md"], "command", "frontmatter", pluginVFrontmatter),
		section(p["commands/sub/deep.md"], "command", "frontmatter", pluginVFrontmatter),
	}
	want := fmt.Sprintf(`{
  "success": false,
  "strict": false,
  "target": %q,
  "manifest": {
    "file": %q,
    "type": "plugin",
    "errors": [],
    "warnings": [],
    "notes": []
  },
  "contents": [
%s
    {
      "file": %q,
      "type": "hooks",
      "errors": [
        {
          "path": "root",
          "message": %q,
          "code": null
        }
      ],
      "warnings": [],
      "notes": []
    }
  ]
}
`,
		filepath.Join(base, ".claude-plugin", "plugin.json"),
		filepath.Join(base, ".claude-plugin", "plugin.json"),
		strings.Join(contents, "\n"),
		p["hooks/hooks.json"], pluginVHooksRoot)
	expectValidate(t, want, true, base, "--json")
}

func TestPluginValidateMarkdownFindings(t *testing.T) {
	t.Run("command-without-description", func(t *testing.T) { // probe Q
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/plugin.json": `{"name":"c","version":"1.0.0","description":"C","author":{"name":"p"}}`,
			"commands/x.md":              "---\nfoo: bar\n---\nbody\n",
		})
		cmd := filepath.Join(base, "commands", "x.md")
		want := fmt.Sprintf("Validating plugin manifest: %s\n\n"+
			"Validating command: %s\n\n⚠ Found 1 warning:\n\n"+
			"  ❯ description: No description in frontmatter. A description helps users and Claude understand when to use this command.\n\n"+
			"✔ Validation passed with warnings\n",
			filepath.Join(base, ".claude-plugin", "plugin.json"), cmd)
		expectValidate(t, want, false, base)
	})
	t.Run("skill-without-description", func(t *testing.T) { // probe R
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/plugin.json": `{"name":"sk","version":"1.0.0","description":"S","author":{"name":"p"}}`,
			"skills/s/SKILL.md":          "---\nname: s\n---\nbody\n",
		})
		skill := filepath.Join(base, "skills", "s", "SKILL.md")
		want := fmt.Sprintf("Validating plugin manifest: %s\n\n"+
			"Validating skill: %s\n\n⚠ Found 1 warning:\n\n"+
			"  ❯ description: No description in frontmatter. A description helps users and Claude understand when to use this skill.\n\n"+
			"✔ Validation passed with warnings\n",
			filepath.Join(base, ".claude-plugin", "plugin.json"), skill)
		expectValidate(t, want, false, base)
	})
	t.Run("skill-without-name-is-clean", func(t *testing.T) { // probe S
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/plugin.json": `{"name":"sk","version":"1.0.0","description":"S","author":{"name":"p"}}`,
			"skills/s/SKILL.md":          "---\ndescription: d\n---\nbody\n",
		})
		pj := filepath.Join(base, ".claude-plugin", "plugin.json")
		expectValidate(t, "Validating plugin manifest: "+pj+"\n\n✔ Validation passed\n", false, base)
	})
	t.Run("deep-skill-not-scanned", func(t *testing.T) { // probe AI
		// Only skills/<name>/SKILL.md counts; a deeper nesting is invisible.
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/plugin.json": `{"name":"ds","version":"1.0.0","description":"D","author":{"name":"p"}}`,
			"skills/a/b/SKILL.md":        "no fm\n",
		})
		pj := filepath.Join(base, ".claude-plugin", "plugin.json")
		expectValidate(t, "Validating plugin manifest: "+pj+"\n\n✔ Validation passed\n", false, base)
	})
	t.Run("deep-agent-scanned", func(t *testing.T) { // probe AJ
		// Agents are walked recursively, unlike skills.
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/plugin.json": `{"name":"da","version":"1.0.0","description":"D","author":{"name":"p"}}`,
			"agents/sub/x.md":            "no fm\n",
		})
		agent := filepath.Join(base, "agents", "sub", "x.md")
		want := fmt.Sprintf("Validating plugin manifest: %s\n\n"+
			"Validating agent: %s\n\n⚠ Found 1 warning:\n\n  ❯ frontmatter: %s\n\n"+
			"✔ Validation passed with warnings\n",
			filepath.Join(base, ".claude-plugin", "plugin.json"), agent, pluginVFrontmatter)
		expectValidate(t, want, false, base)
	})
}

// hookFixture builds a plugin with the given hooks/hooks.json content.
func hookFixture(t *testing.T, hooksJSON string) (base, pj, hooks string) {
	t.Helper()
	base = writeValidateFiles(t, t.TempDir(), map[string]string{
		".claude-plugin/plugin.json": `{"name":"hm","version":"1.0.0","description":"H","author":{"name":"p"}}`,
		"hooks/hooks.json":           hooksJSON,
	})
	return base, filepath.Join(base, ".claude-plugin", "plugin.json"),
		filepath.Join(base, "hooks", "hooks.json")
}

func TestPluginValidateHooks(t *testing.T) {
	t.Run("valid-hooks-not-reported", func(t *testing.T) { // probe M
		base, pj, _ := hookFixture(t, `{"hooks": {}}`)
		expectValidate(t, "Validating plugin manifest: "+pj+"\n\n✔ Validation passed\n", false, base)
	})
	t.Run("modules-object", func(t *testing.T) { // probe AF
		base, pj, hooks := hookFixture(t, `{"modules": {}}`)
		want := fmt.Sprintf("Validating plugin manifest: %s\n\n"+
			"Validating hooks: %s\n\n✘ Found 1 error:\n\n  ❯ modules: Invalid input\n\n✘ Validation failed\n", pj, hooks)
		expectValidate(t, want, true, base)
	})
	t.Run("hooks-and-modules-object", func(t *testing.T) { // probe AG
		base, pj, hooks := hookFixture(t, `{"hooks": {}, "modules": {}}`)
		want := fmt.Sprintf("Validating plugin manifest: %s\n\n"+
			"Validating hooks: %s\n\n✘ Found 1 error:\n\n  ❯ modules: Invalid input\n\n✘ Validation failed\n", pj, hooks)
		expectValidate(t, want, true, base)
	})
	t.Run("empty-modules-array", func(t *testing.T) { // probe AN
		base, pj, hooks := hookFixture(t, `{"modules": []}`)
		want := fmt.Sprintf("Validating plugin manifest: %s\n\n"+
			"Validating hooks: %s\n\n✘ Found 1 error:\n\n  ❯ root: %s\n\n✘ Validation failed\n", pj, hooks, pluginVHooksRoot)
		expectValidate(t, want, true, base)
	})
	t.Run("missing-module-file", func(t *testing.T) { // probe AO
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			// Manifest name deliberately differs from the directory name: the
			// oracle prefixes the error with the plugin root's directory name.
			".claude-plugin/plugin.json": `{"name":"hm3","version":"1.0.0","description":"H","author":{"name":"p"}}`,
			"hooks/hooks.json":           `{"modules": ["m.js"]}`,
		})
		pj := filepath.Join(base, ".claude-plugin", "plugin.json")
		hooks := filepath.Join(base, "hooks", "hooks.json")
		want := fmt.Sprintf("Validating plugin manifest: %s\n\n"+
			"Validating hooks: %s\n\n✘ Found 1 error:\n\n"+
			"  ❯ modules.m.js: %s: %s: no such file\n\n✘ Validation failed\n",
			pj, hooks, filepath.Base(base), filepath.Join(base, "hooks", "m.js"))
		expectValidate(t, want, true, base)
	})
	t.Run("unparseable-hooks-json", func(t *testing.T) { // probe AH
		base, pj, hooks := hookFixture(t, "not json\n")
		want := fmt.Sprintf("Validating plugin manifest: %s\n\n"+
			"Validating hooks: %s\n\n✘ Found 1 error:\n\n"+
			"  ❯ json: Invalid JSON syntax: JSON Parse error: invalid character 'o' in literal null (expecting 'u')"+
			" At runtime this breaks the entire plugin load.\n\n✘ Validation failed\n", pj, hooks)
		expectValidate(t, want, true, base)
	})
}

func TestPluginValidateMarketplace(t *testing.T) {
	t.Run("mpw-text", func(t *testing.T) { // probe A
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/marketplace.json": `{"name":"mpw","owner":{"name":"p"},"plugins":[` +
				`{"name":"p1","source":"./plugins/p1","version":"1.0.0","description":"P1"},` +
				`{"name":"p2","source":"./plugins/p2","version":"1.0.0","description":"P2","author":{"name":"p"}}]}`,
			"plugins/p1/.claude-plugin/plugin.json": `{"name":"p1","version":"1.0.0","description":"P1"}`,
			"plugins/p2/.claude-plugin/plugin.json": `{"name":"p2","version":"1.0.0","description":"P2","author":{"name":"p"}}`,
		})
		mp := filepath.Join(base, ".claude-plugin", "marketplace.json")
		want := fmt.Sprintf("Validating marketplace manifest: %s\n\n"+
			"⚠ Found 2 warnings:\n\n"+
			"  ❯ description: %s\n"+
			"  ❯ plugins[0] plugin.json → author: %s\n\n"+
			"✔ Validation passed with warnings\n", mp, pluginVNoMPDesc, pluginVNoAuthor)
		expectValidate(t, want, false, base)
	})
	t.Run("mpw-json", func(t *testing.T) { // probe B
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/marketplace.json": `{"name":"mpw","owner":{"name":"p"},"plugins":[` +
				`{"name":"p1","source":"./plugins/p1","version":"1.0.0","description":"P1"},` +
				`{"name":"p2","source":"./plugins/p2","version":"1.0.0","description":"P2","author":{"name":"p"}}]}`,
			"plugins/p1/.claude-plugin/plugin.json": `{"name":"p1","version":"1.0.0","description":"P1"}`,
			"plugins/p2/.claude-plugin/plugin.json": `{"name":"p2","version":"1.0.0","description":"P2","author":{"name":"p"}}`,
		})
		mp := filepath.Join(base, ".claude-plugin", "marketplace.json")
		want := fmt.Sprintf(`{
  "success": true,
  "strict": false,
  "target": %q,
  "manifest": {
    "file": %q,
    "type": "marketplace",
    "errors": [],
    "warnings": [
      {
        "path": "description",
        "message": %q,
        "code": null
      },
      {
        "path": "plugins[0] plugin.json → author",
        "message": %q,
        "code": null
      }
    ],
    "notes": []
  },
  "contents": []
}
`, mp, mp, pluginVNoMPDesc, pluginVNoAuthor)
		expectValidate(t, want, false, base, "--json")
	})
	t.Run("mp-file-and-dir-target", func(t *testing.T) { // probes H + AB
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/marketplace.json": `{"name":"mp1","owner":{"name":"probe"},"plugins":[` +
				`{"name":"myplugin","source":"./plugins/myplugin","description":"A probe plugin","version":"2.0.0"},` +
				`{"name":"rich","source":"./plugins/rich","description":"Rich plugin","version":"2.0.0","author":{"name":"probe"}}],` +
				`"description":"Probe marketplace"}`,
			"plugins/myplugin/.claude-plugin/plugin.json": `{"name":"myplugin","version":"9.9.9","description":"A probe plugin"}`,
			"plugins/rich/.claude-plugin/plugin.json":     `{"name":"rich","version":"2.0.0","description":"Rich plugin","author":{"name":"probe"}}`,
		})
		mp := filepath.Join(base, ".claude-plugin", "marketplace.json")
		want := fmt.Sprintf("Validating marketplace manifest: %s\n\n"+
			"⚠ Found 2 warnings:\n\n"+
			"  ❯ plugins[0].version: %s\n"+
			"  ❯ plugins[0] plugin.json → author: %s\n\n"+
			"✔ Validation passed with warnings\n", mp,
			pluginVMismatch("2.0.0", "plugins/myplugin/.claude-plugin/plugin.json", "9.9.9"), pluginVNoAuthor)
		expectValidate(t, want, false, mp)
		expectValidate(t, want, false, base)
	})
}

func TestPluginValidateMarketplaceEntries(t *testing.T) {
	cases := []struct {
		name   string
		mpJSON string
		want   string
		fail   bool
	}{
		{"entry-without-name", // probe T
			`{"name":"e1","owner":{"name":"p"},"description":"D","plugins":[{"source":"./x"}]}`,
			"  ❯ plugins.0.name: Invalid input\n", true},
		{"entry-without-source", // probe U
			`{"name":"e2","owner":{"name":"p"},"description":"D","plugins":[{"name":"x"}]}`,
			"  ❯ plugins.0.source: Invalid input\n", true},
		{"entry-not-an-object", // probe V
			`{"name":"e3","owner":{"name":"p"},"description":"D","plugins":["notanobject"]}`,
			"  ❯ plugins.0: Invalid input\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := writeValidateFiles(t, t.TempDir(), map[string]string{
				".claude-plugin/marketplace.json": tc.mpJSON,
			})
			mp := filepath.Join(base, ".claude-plugin", "marketplace.json")
			want := "Validating marketplace manifest: " + mp + "\n\n✘ Found 1 error:\n\n" + tc.want + "\n✘ Validation failed\n"
			expectValidate(t, want, tc.fail, base)
		})
	}
	t.Run("entry-without-version-is-clean", func(t *testing.T) { // probe W
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/marketplace.json": `{"name":"e4","owner":{"name":"p"},"description":"D","plugins":[` +
				`{"name":"p1","source":"./plugins/p1","description":"D"}]}`,
			"plugins/p1/.claude-plugin/plugin.json": `{"name":"p1","version":"1.0.0","description":"P","author":{"name":"p"}}`,
		})
		mp := filepath.Join(base, ".claude-plugin", "marketplace.json")
		expectValidate(t, "Validating marketplace manifest: "+mp+"\n\n✔ Validation passed\n", false, base)
	})
	t.Run("entry-without-description-is-clean", func(t *testing.T) { // probe X
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/marketplace.json": `{"name":"e5","owner":{"name":"p"},"description":"D","plugins":[` +
				`{"name":"p1","source":"./plugins/p1","version":"1.0.0"}]}`,
			"plugins/p1/.claude-plugin/plugin.json": `{"name":"p1","version":"1.0.0","description":"P","author":{"name":"p"}}`,
		})
		mp := filepath.Join(base, ".claude-plugin", "marketplace.json")
		expectValidate(t, "Validating marketplace manifest: "+mp+"\n\n✔ Validation passed\n", false, base)
	})
	t.Run("incomplete-plugin-json-three-warnings", func(t *testing.T) { // probe AL
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/marketplace.json": `{"name":"bs","owner":{"name":"p"},"description":"D","plugins":[` +
				`{"name":"bad","source":"./plugins/bad","version":"1.0.0","description":"B"}]}`,
			"plugins/bad/.claude-plugin/plugin.json": `{"name":"bad"}`,
		})
		mp := filepath.Join(base, ".claude-plugin", "marketplace.json")
		want := fmt.Sprintf("Validating marketplace manifest: %s\n\n"+
			"⚠ Found 3 warnings:\n\n"+
			"  ❯ plugins[0] plugin.json → version: %s\n"+
			"  ❯ plugins[0] plugin.json → description: %s\n"+
			"  ❯ plugins[0] plugin.json → author: %s\n\n"+
			"✔ Validation passed with warnings\n", mp, pluginVNoVersion, pluginVNoDescription, pluginVNoAuthor)
		expectValidate(t, want, false, base)
	})
	t.Run("plugin-json-without-version", func(t *testing.T) { // probe AM
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/marketplace.json": `{"name":"nv","owner":{"name":"p"},"description":"D","plugins":[` +
				`{"name":"p","source":"./plugins/p","version":"1.0.0","description":"P"}]}`,
			"plugins/p/.claude-plugin/plugin.json": `{"name":"p","description":"P"}`,
		})
		mp := filepath.Join(base, ".claude-plugin", "marketplace.json")
		want := fmt.Sprintf("Validating marketplace manifest: %s\n\n"+
			"⚠ Found 2 warnings:\n\n"+
			"  ❯ plugins[0] plugin.json → version: %s\n"+
			"  ❯ plugins[0] plugin.json → author: %s\n\n"+
			"✔ Validation passed with warnings\n", mp, pluginVNoVersion, pluginVNoAuthor)
		expectValidate(t, want, false, base)
	})
}

func TestPluginValidateMarketplaceEdges(t *testing.T) {
	t.Run("string-owner", func(t *testing.T) { // probe P
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/marketplace.json": `{"name":"strowner","owner":"somestring","description":"S","plugins":[]}`,
		})
		mp := filepath.Join(base, ".claude-plugin", "marketplace.json")
		want := "Validating marketplace manifest: " + mp + "\n\n" +
			"✘ Found 1 error:\n\n  ❯ owner: Invalid input\n\n✘ Validation failed\n"
		expectValidate(t, want, true, base)
	})
	t.Run("string-owner-json", func(t *testing.T) { // probe I
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/marketplace.json": `{"name":"strowner","owner":"somestring","description":"S","plugins":[]}`,
		})
		mp := filepath.Join(base, ".claude-plugin", "marketplace.json")
		want := fmt.Sprintf(`{
  "success": false,
  "strict": false,
  "target": %q,
  "manifest": {
    "file": %q,
    "type": "marketplace",
    "errors": [
      {
        "path": "owner",
        "message": "Invalid input",
        "code": null
      }
    ],
    "warnings": [],
    "notes": []
  },
  "contents": []
}
`, mp, mp)
		expectValidate(t, want, true, base, "--json")
	})
	t.Run("missing-plugins-key", func(t *testing.T) { // probe AK
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/marketplace.json": `{"name":"noplugins","owner":{"name":"p"}}`,
		})
		mp := filepath.Join(base, ".claude-plugin", "marketplace.json")
		want := "Validating marketplace manifest: " + mp + "\n\n" +
			"✘ Found 1 error:\n\n  ❯ plugins: Invalid input\n\n✘ Validation failed\n"
		expectValidate(t, want, true, mp)
	})
	t.Run("empty-plugins-list", func(t *testing.T) { // probe AE
		// A directory with both manifests prefers the marketplace one.
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			".claude-plugin/marketplace.json": `{"name":"bmp","owner":{"name":"p"},"description":"B","plugins":[]}`,
			".claude-plugin/plugin.json":      `{"name":"bplugin","version":"1.0.0","description":"B","author":{"name":"p"}}`,
		})
		mp := filepath.Join(base, ".claude-plugin", "marketplace.json")
		want := "Validating marketplace manifest: " + mp + "\n\n" +
			"⚠ Found 1 warning:\n\n  ❯ plugins: Marketplace has no plugins defined\n\n" +
			"✔ Validation passed with warnings\n"
		expectValidate(t, want, false, base)
	})
	t.Run("loose-marketplace-file-target", func(t *testing.T) { // probe N
		// Entry sources resolve against the loose file's own directory, where
		// they do not exist, so no per-entry warnings are produced.
		base := writeValidateFiles(t, t.TempDir(), map[string]string{
			"other.json": `{"name":"mp1","owner":{"name":"probe"},"plugins":[` +
				`{"name":"myplugin","source":"./plugins/myplugin","description":"A probe plugin","version":"2.0.0"},` +
				`{"name":"rich","source":"./plugins/rich","description":"Rich plugin","version":"2.0.0","author":{"name":"probe"}}],` +
				`"description":"Probe marketplace"}`,
		})
		other := filepath.Join(base, "other.json")
		expectValidate(t, "Validating marketplace manifest: "+other+"\n\n✔ Validation passed\n", false, other)
	})
	t.Run("root-derivation-file-and-dir", func(t *testing.T) { // probes AC + AD
		// A loose marketplace file resolves entries against its own dir; the
		// in-place one against the parent of .claude-plugin.  Same warnings.
		pj := `{"name":"p","version":"1.0.0","description":"P"}`
		fileBase := writeValidateFiles(t, t.TempDir(), map[string]string{
			"plugins/p/.claude-plugin/plugin.json": pj,
			"x.json":                               `{"name":"rt","owner":{"name":"p"},"description":"D","plugins":[{"name":"p","source":"./plugins/p","version":"2.0.0","description":"P"}]}`,
		})
		dirBase := writeValidateFiles(t, t.TempDir(), map[string]string{
			"plugins/p/.claude-plugin/plugin.json": pj,
			".claude-plugin/marketplace.json":      `{"name":"rt2","owner":{"name":"p"},"description":"D","plugins":[{"name":"p","source":"./plugins/p","version":"2.0.0","description":"P"}]}`,
		})
		wantFor := func(mp string) string {
			return fmt.Sprintf("Validating marketplace manifest: %s\n\n"+
				"⚠ Found 2 warnings:\n\n"+
				"  ❯ plugins[0].version: %s\n"+
				"  ❯ plugins[0] plugin.json → author: %s\n\n"+
				"✔ Validation passed with warnings\n", mp,
				pluginVMismatch("2.0.0", "plugins/p/.claude-plugin/plugin.json", "1.0.0"), pluginVNoAuthor)
		}
		expectValidate(t, wantFor(filepath.Join(fileBase, "x.json")), false, filepath.Join(fileBase, "x.json"))
		expectValidate(t, wantFor(filepath.Join(dirBase, ".claude-plugin", "marketplace.json")), false, dirBase)
	})
}

func TestPluginValidateMissingTargets(t *testing.T) {
	// Unverified edges (no oracle capture): the below shapes are this build's
	// invention, pinned as regression tests.
	t.Run("nonexistent-path", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "no-such-plugin")
		want := "Validating plugin manifest: " + missing + "\n\n" +
			"✘ Found 1 error:\n\n  ❯ file: File not found: " + missing + "\n\n✘ Validation failed\n"
		expectValidate(t, want, true, missing)
	})
	t.Run("directory-without-manifest", func(t *testing.T) {
		empty := t.TempDir()
		want := "Validating plugin manifest: " + empty + "\n\n" +
			"✘ Found 1 error:\n\n  ❯ directory: No manifest found in directory. " +
			"Expected .claude-plugin/marketplace.json or .claude-plugin/plugin.json\n\n✘ Validation failed\n"
		expectValidate(t, want, true, empty)
	})
}
