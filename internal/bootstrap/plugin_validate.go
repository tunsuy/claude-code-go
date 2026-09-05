package bootstrap

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// `claude plugin validate <path>` — local-only validation of a plugin or
// marketplace manifest plus the plugin's contents, byte-pinned against the
// oracle (claude v2.1.261; see the probes in internal/bootstrap/CONTEXT.md).
//
// Rules (all oracle-verified unless noted):
//   - A directory prefers .claude-plugin/marketplace.json over plugin.json;
//     neither is an error. A file target is parsed as JSON and classified by
//     content: an "owner" key means marketplace, else plugin.
//   - The marketplace root (what entry sources resolve against) is the parent
//     of .claude-plugin for in-place manifests, else the manifest's own dir.
//   - Schema errors ("Invalid input") suppress the semantic warnings.
//   - Contents are only scanned for plugin manifests at <root>/.claude-plugin/
//     plugin.json: skills/*/SKILL.md (one level), then agents/**/*.md, then
//     commands/**/*.md (both recursive), then hooks/hooks.json — in that group
//     order, path-sorted within each group. Only files with findings appear.
//   - --strict flips the verdict (and exit code) when warnings alone exist;
//     real errors already fail and get no strict suffix.
//
// Documented divergence: the oracle's JS JSON parser words parse errors its
// own way ("Unexpected identifier \"not\""); Go's wording differs but the
// wrapping format matches. Module files under hooks/ that exist are not
// loaded or parsed (the oracle reads them as JS).

// pluginFinding is one error or warning in a validation report.
type pluginFinding struct {
	Path    string `json:"path"`
	Message string `json:"message"`
	Code    any    `json:"code"` // always null in the oracle's output
}

// pluginSectionReport is the findings for the manifest or one content file.
type pluginSectionReport struct {
	File     string          `json:"file"`
	Type     string          `json:"type"`
	Errors   []pluginFinding `json:"errors"`
	Warnings []pluginFinding `json:"warnings"`
	Notes    []pluginFinding `json:"notes"`
}

// pluginValidateReport is the --json document.
type pluginValidateReport struct {
	Success  bool                  `json:"success"`
	Strict   bool                  `json:"strict"`
	Target   string                `json:"target"`
	Manifest pluginSectionReport   `json:"manifest"`
	Contents []pluginSectionReport `json:"contents"`
}

// runPluginValidate implements `plugin validate`.  Everything prints to
// stdout (the oracle never uses stderr here); failures exit 1 silently.
func runPluginValidate(d *pluginDeps, args []string) error {
	p, err := parseMCPFlags(args, pluginValidateFlagSpecs)
	if err != nil {
		return err
	}
	if p.SawHelp {
		fmt.Fprint(d.Stdout, pluginValidateHelp)
		return nil
	}
	if err := mcpCheckPositionals(p, pluginPathArg); err != nil {
		return err
	}

	rep := pluginValidateTarget(p.Positionals[0])
	pluginNormalizeSections(rep)
	rep.Strict = p.Bools["strict"]
	failed := pluginValidateFailed(rep)
	rep.Success = !failed

	if p.Bools["json"] {
		if err := pluginWriteJSON(d, rep); err != nil {
			return err
		}
	} else {
		pluginRenderValidateText(d, rep)
	}
	if failed {
		return &errMCPExit{code: 1}
	}
	return nil
}

// pluginNormalizeSections fills nil finding slices so the JSON form renders
// them as [] rather than null (oracle-verified: every section always carries
// all three arrays).
func pluginNormalizeSections(rep *pluginValidateReport) {
	for _, s := range append([]*pluginSectionReport{&rep.Manifest}, sectionPtrs(rep.Contents)...) {
		if s.Errors == nil {
			s.Errors = []pluginFinding{}
		}
		if s.Warnings == nil {
			s.Warnings = []pluginFinding{}
		}
		if s.Notes == nil {
			s.Notes = []pluginFinding{}
		}
	}
}

// sectionPtrs addresses each content section for in-place normalization.
func sectionPtrs(sections []pluginSectionReport) []*pluginSectionReport {
	out := make([]*pluginSectionReport, len(sections))
	for i := range sections {
		out[i] = &sections[i]
	}
	return out
}

// pluginValidateFailed reports the overall outcome: any real error fails;
// --strict additionally fails warning-only runs.
func pluginValidateFailed(rep *pluginValidateReport) bool {
	errN, warnN := 0, 0
	for _, s := range append([]pluginSectionReport{rep.Manifest}, rep.Contents...) {
		errN += len(s.Errors)
		warnN += len(s.Warnings)
	}
	return errN > 0 || (rep.Strict && warnN > 0)
}

// pluginValidateTarget resolves the target path to a report: the manifest
// section plus, for plugin manifests, the content sections.
func pluginValidateTarget(target string) *pluginValidateReport {
	rep := &pluginValidateReport{Target: target, Contents: []pluginSectionReport{}}
	info, err := os.Stat(target)
	if err != nil {
		rep.Manifest = pluginSectionReport{
			File:   target,
			Type:   "plugin",
			Errors: []pluginFinding{{Path: "file", Message: "File not found: " + target}},
		}
		return rep
	}
	if !info.IsDir() {
		pluginValidateManifestFile(target, rep)
		return rep
	}

	// A directory prefers the marketplace manifest; a bare manifest path
	// that does not exist falls back to plugin.json, then to the miss error.
	if mpPath := filepath.Join(target, ".claude-plugin", "marketplace.json"); pluginFileExists(mpPath) {
		pluginValidateManifestFile(mpPath, rep)
		return rep
	}
	if pjPath := filepath.Join(target, ".claude-plugin", "plugin.json"); pluginFileExists(pjPath) {
		pluginValidateManifestFile(pjPath, rep)
		return rep
	}
	rep.Manifest = pluginSectionReport{
		File: target,
		Type: "plugin",
		Errors: []pluginFinding{{
			Path: "directory",
			Message: "No manifest found in directory. Expected .claude-plugin/marketplace.json " +
				"or .claude-plugin/plugin.json",
		}},
	}
	return rep
}

// pluginValidateManifestFile validates one manifest file (by content) and,
// for plugins at <root>/.claude-plugin/plugin.json, scans the contents.
func pluginValidateManifestFile(path string, rep *pluginValidateReport) {
	raw, err := pluginReadJSONObject(path)
	if err != nil {
		rep.Manifest = pluginSectionReport{
			File:   path,
			Type:   "plugin",
			Errors: []pluginFinding{{Path: "json", Message: err.Error()}},
		}
		return
	}

	_, isMarketplace := raw["owner"]
	section := pluginSectionReport{File: path, Errors: []pluginFinding{}, Warnings: []pluginFinding{}, Notes: []pluginFinding{}}
	if isMarketplace {
		section.Type = "marketplace"
		pluginValidateMarketplace(raw, &section)
	} else {
		section.Type = "plugin"
		pluginValidatePluginManifest(raw, &section)
	}
	rep.Manifest = section
	rep.Target = path

	if section.Type != "plugin" {
		return // marketplaces never scan contents
	}
	// Contents only for the canonical layout: a bare manifest file elsewhere
	// (e.g. a loose x.json) has no plugin root to walk.
	dir := filepath.Dir(path)
	if filepath.Base(dir) != ".claude-plugin" || filepath.Base(path) != "plugin.json" {
		return
	}
	root := filepath.Dir(dir)
	pluginScanContents(root, rep)
}

// pluginValidateMarketplace runs the marketplace schema checks and, when the
// schema passes, the semantic warnings (including per-entry plugin.json
// checks — the oracle reads each entry's source).
func pluginValidateMarketplace(raw map[string]any, section *pluginSectionReport) {
	if !pluginNonEmptyString(raw["name"]) {
		section.Errors = append(section.Errors, pluginFinding{Path: "name", Message: "Invalid input"})
	}
	if !pluginIsMap(raw["owner"]) {
		section.Errors = append(section.Errors, pluginFinding{Path: "owner", Message: "Invalid input"})
	}
	entries := []map[string]any{}
	pluginsRaw, hasPlugins := raw["plugins"]
	if hasPlugins {
		if arr, ok := pluginsRaw.([]any); ok {
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					entries = append(entries, m)
				} else {
					entries = append(entries, nil) // placeholder keeps indices aligned
				}
			}
		} else {
			hasPlugins = false // wrong type: report as missing
		}
	}
	if !hasPlugins {
		section.Errors = append(section.Errors, pluginFinding{Path: "plugins", Message: "Invalid input"})
	} else {
		for i, e := range entries {
			if e == nil {
				section.Errors = append(section.Errors,
					pluginFinding{Path: fmt.Sprintf("plugins.%d", i), Message: "Invalid input"})
				continue
			}
			if !pluginNonEmptyString(e["name"]) {
				section.Errors = append(section.Errors,
					pluginFinding{Path: fmt.Sprintf("plugins.%d.name", i), Message: "Invalid input"})
			}
			if !pluginNonEmptyString(e["source"]) {
				section.Errors = append(section.Errors,
					pluginFinding{Path: fmt.Sprintf("plugins.%d.source", i), Message: "Invalid input"})
			}
		}
	}
	if len(section.Errors) > 0 {
		return // schema failures suppress the semantic checks
	}

	if !pluginNonEmptyString(raw["description"]) {
		section.Warnings = append(section.Warnings, pluginFinding{
			Path: "description",
			Message: "No marketplace description provided. Adding a description helps users " +
				"understand what this marketplace offers",
		})
	}
	if len(entries) == 0 {
		section.Warnings = append(section.Warnings,
			pluginFinding{Path: "plugins", Message: "Marketplace has no plugins defined"})
	}
	root := marketplaceRootFor(section.File)
	for i, e := range entries {
		source, _ := e["source"].(string)
		srcDir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(source, "./")))
		pj, err := pluginReadJSONObject(filepath.Join(srcDir, ".claude-plugin", "plugin.json"))
		if err != nil || pj == nil {
			continue // unresolvable sources are skipped silently (oracle-verified)
		}
		entryVersion, _ := e["version"].(string)
		pjVersion, _ := pj["version"].(string)
		rel := strings.TrimPrefix(source, "./") + "/.claude-plugin/plugin.json"
		if entryVersion != "" && pjVersion != "" && entryVersion != pjVersion {
			section.Warnings = append(section.Warnings, pluginFinding{
				Path: fmt.Sprintf("plugins[%d].version", i),
				Message: fmt.Sprintf("Entry declares version %q but %s says %q. At install time, "+
					"plugin.json wins (calculatePluginVersion precedence) — the entry version is "+
					"silently ignored. Update this entry to %q to match.", entryVersion, rel, pjVersion, pjVersion),
			})
		}
		prefix := fmt.Sprintf("plugins[%d] plugin.json → ", i)
		if pjVersion == "" {
			section.Warnings = append(section.Warnings, pluginFinding{
				Path: prefix + "version",
				Message: "No version specified. Consider adding a version following semver " +
					"(e.g., \"1.0.0\")",
			})
		}
		if !pluginNonEmptyString(pj["description"]) {
			section.Warnings = append(section.Warnings, pluginFinding{
				Path:    prefix + "description",
				Message: "No description provided. Adding a description helps users understand what your plugin does",
			})
		}
		if !pluginHasAuthor(pj["author"]) {
			section.Warnings = append(section.Warnings, pluginFinding{
				Path:    prefix + "author",
				Message: "No author information provided. Consider adding author details for plugin attribution",
			})
		}
	}
}

// pluginValidatePluginManifest runs the plugin.json checks.
func pluginValidatePluginManifest(raw map[string]any, section *pluginSectionReport) {
	if !pluginNonEmptyString(raw["name"]) {
		section.Errors = append(section.Errors, pluginFinding{Path: "name", Message: "Invalid input"})
		return
	}
	if !pluginNonEmptyString(raw["version"]) {
		section.Warnings = append(section.Warnings, pluginFinding{
			Path:    "version",
			Message: "No version specified. Consider adding a version following semver (e.g., \"1.0.0\")",
		})
	}
	if !pluginNonEmptyString(raw["description"]) {
		section.Warnings = append(section.Warnings, pluginFinding{
			Path:    "description",
			Message: "No description provided. Adding a description helps users understand what your plugin does",
		})
	}
	if !pluginHasAuthor(raw["author"]) {
		section.Warnings = append(section.Warnings, pluginFinding{
			Path:    "author",
			Message: "No author information provided. Consider adding author details for plugin attribution",
		})
	}
}

// pluginScanContents walks the four content groups in the oracle's order
// (skills, agents, commands, hooks), path-sorted within each group, and
// appends a section per file that has findings.  Hook module errors are
// prefixed with the plugin root's directory name — oracle-verified: the
// hookmod3 probe (dir hookmod3, manifest name hm3) reports "hookmod3:".
func pluginScanContents(root string, rep *pluginValidateReport) {
	appendSection := func(file, kind string, warnings, errors []pluginFinding) {
		if len(warnings) == 0 && len(errors) == 0 {
			return // clean files are not reported at all
		}
		rep.Contents = append(rep.Contents, pluginSectionReport{
			File:     file,
			Type:     kind,
			Errors:   errors,
			Warnings: warnings,
			Notes:    []pluginFinding{},
		})
	}

	// skills/<name>/SKILL.md — exactly one level, no recursion.
	for _, file := range pluginListFiles(filepath.Join(root, "skills"), false, "SKILL.md") {
		w, e := pluginCheckMarkdown(file, "skill")
		appendSection(file, "skill", w, e)
	}
	// agents/**/*.md and commands/**/*.md — recursive.
	for _, file := range pluginListFiles(filepath.Join(root, "agents"), true, ".md") {
		w, e := pluginCheckMarkdown(file, "agent")
		appendSection(file, "agent", w, e)
	}
	for _, file := range pluginListFiles(filepath.Join(root, "commands"), true, ".md") {
		w, e := pluginCheckMarkdown(file, "command")
		appendSection(file, "command", w, e)
	}
	// hooks/hooks.json.
	hooksPath := filepath.Join(root, "hooks", "hooks.json")
	if pluginFileExists(hooksPath) {
		w, e := pluginCheckHooks(hooksPath, root, filepath.Base(root))
		appendSection(hooksPath, "hooks", w, e)
	}
}

// pluginListFiles collects files named base (or with suffix base when
// recursive) under dir, in path order. With recursive=false only direct
// children matching base exactly are listed (the skills/*/SKILL.md shape).
func pluginListFiles(dir string, recursive bool, base string) []string {
	var out []string
	if !recursive {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		for _, e := range entries {
			if e.IsDir() {
				if f := filepath.Join(dir, e.Name(), base); pluginFileExists(f) {
					out = append(out, f)
				}
			}
		}
		return out
	}
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable subtrees are skipped
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), base) {
			out = append(out, path)
		}
		return nil
	})
	return out
}

// pluginCheckMarkdown validates a command, skill or agent markdown file:
// frontmatter presence, then a description inside it.  Returns warnings
// only (errors is always empty; kept for the appendSection signature).
func pluginCheckMarkdown(file, kind string) ([]pluginFinding, []pluginFinding) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, nil
	}
	fm, hasBlock := pluginFrontmatter(data)
	if !hasBlock {
		return []pluginFinding{{
			Path: "frontmatter",
			Message: "No frontmatter block found. Add YAML frontmatter between --- delimiters at " +
				"the top of the file to set description and other metadata.",
		}}, nil
	}
	if fm["description"] == "" {
		return []pluginFinding{{
			Path:    "description",
			Message: fmt.Sprintf("No description in frontmatter. A description helps users and Claude understand when to use this %s.", kind),
		}}, nil
	}
	return nil, nil
}

// pluginFrontmatter extracts the frontmatter block's keys and reports
// whether a --- ... --- delimited block exists at the top of data.  The
// oracle needs only the description key, so a minimal line parser suffices.
func pluginFrontmatter(data []byte) (map[string]string, bool) {
	lines := strings.Split(string(data), "\n")
	fm := map[string]string{}
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return fm, false
	}
	for _, l := range lines[1:] {
		l = strings.TrimRight(l, "\r")
		if l == "---" {
			return fm, true
		}
		if k, v, ok := strings.Cut(l, ":"); ok {
			fm[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return fm, false
}

// pluginCheckHooks validates hooks/hooks.json: JSON parse, then the
// hooks/modules shape, then module file resolution under hooks/.
func pluginCheckHooks(file, root, pluginName string) ([]pluginFinding, []pluginFinding) {
	var errors []pluginFinding
	raw, err := pluginReadJSONObject(file)
	if err != nil {
		return nil, append(errors, pluginFinding{
			Path:    "json",
			Message: err.Error() + " At runtime this breaks the entire plugin load.",
		})
	}

	hooksCounts := false
	if v, ok := raw["hooks"]; ok {
		if _, isMap := v.(map[string]any); isMap {
			hooksCounts = true
		} else {
			errors = append(errors, pluginFinding{Path: "hooks", Message: "Invalid input"})
		}
	}
	modulesCounts := false
	if v, ok := raw["modules"]; ok {
		arr, isArr := v.([]any)
		if !isArr {
			errors = append(errors, pluginFinding{Path: "modules", Message: "Invalid input"})
		} else if len(arr) > 0 {
			allStrings := true
			for _, m := range arr {
				if _, ok := m.(string); !ok {
					allStrings = false
					break
				}
			}
			if !allStrings {
				errors = append(errors, pluginFinding{Path: "modules", Message: "Invalid input"})
			} else {
				modulesCounts = true
				for _, m := range arr {
					path := filepath.Join(root, "hooks", m.(string))
					if !pluginFileExists(path) {
						errors = append(errors, pluginFinding{
							Path:    "modules." + m.(string),
							Message: fmt.Sprintf("%s: %s: no such file", pluginName, path),
						})
					}
				}
			}
		}
	}
	if len(errors) == 0 && !hooksCounts && !modulesCounts {
		errors = append(errors, pluginFinding{
			Path: "root",
			Message: "hooks.json must have `hooks` (the hook matchers) or `modules` " +
				"(hooks modules), or both",
		})
	}
	return nil, errors
}

// pluginRenderValidateText prints the text report: a header per section,
// findings blocks, then the verdict.
func pluginRenderValidateText(d *pluginDeps, rep *pluginValidateReport) {
	var b strings.Builder
	fmt.Fprintf(&b, "Validating %s manifest: %s\n\n", rep.Manifest.Type, rep.Manifest.File)
	pluginWriteFindings(&b, &rep.Manifest)
	for i := range rep.Contents {
		fmt.Fprintf(&b, "Validating %s: %s\n\n", rep.Contents[i].Type, rep.Contents[i].File)
		pluginWriteFindings(&b, &rep.Contents[i])
	}

	errN, warnN := 0, 0
	for _, s := range append([]pluginSectionReport{rep.Manifest}, rep.Contents...) {
		errN += len(s.Errors)
		warnN += len(s.Warnings)
	}
	switch {
	case errN > 0:
		b.WriteString("✘ Validation failed\n")
	case warnN > 0 && rep.Strict:
		b.WriteString("✘ Validation failed (--strict treats warnings as errors)\n")
	case warnN > 0:
		b.WriteString("✔ Validation passed with warnings\n")
	default:
		b.WriteString("✔ Validation passed\n")
	}
	fmt.Fprint(d.Stdout, b.String())
}

// pluginWriteFindings prints one section's warnings then errors, each as its
// own ⚠/✘ block.
func pluginWriteFindings(b *strings.Builder, s *pluginSectionReport) {
	if len(s.Warnings) > 0 {
		fmt.Fprintf(b, "⚠ Found %d %s:\n\n", len(s.Warnings), pluginCountNoun(len(s.Warnings), "warning"))
		for _, f := range s.Warnings {
			fmt.Fprintf(b, "  ❯ %s: %s\n", f.Path, f.Message)
		}
		b.WriteString("\n")
	}
	if len(s.Errors) > 0 {
		fmt.Fprintf(b, "✘ Found %d %s:\n\n", len(s.Errors), pluginCountNoun(len(s.Errors), "error"))
		for _, f := range s.Errors {
			fmt.Fprintf(b, "  ❯ %s: %s\n", f.Path, f.Message)
		}
		b.WriteString("\n")
	}
}

// pluginCountNoun renders "warning"/"warnings"-style counts.
func pluginCountNoun(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// marketplaceRootFor returns what a manifest's entry sources resolve against:
// the parent of .claude-plugin when the file lives in one, else its own dir.
func marketplaceRootFor(manifestPath string) string {
	dir := filepath.Dir(manifestPath)
	if filepath.Base(dir) == ".claude-plugin" {
		return filepath.Dir(dir)
	}
	return dir
}

// pluginReadJSONObject reads and parses a JSON object; a parse failure is
// returned already worded as the oracle's json error. A missing file
// returns (nil, nil).
func pluginReadJSONObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin validate: read %s: %w", path, err)
	}
	raw := map[string]any{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("Invalid JSON syntax: JSON Parse error: %s", err.Error())
	}
	return raw, nil
}

// pluginFileExists reports whether path is a regular file.
func pluginFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// pluginNonEmptyString reports a non-empty string value.
func pluginNonEmptyString(v any) bool {
	s, ok := v.(string)
	return ok && s != ""
}

// pluginIsMap reports an object value.
func pluginIsMap(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}

// pluginHasAuthor reports a usable author field: a non-empty string or a
// non-empty object.
func pluginHasAuthor(v any) bool {
	switch t := v.(type) {
	case string:
		return t != ""
	case map[string]any:
		return len(t) > 0
	}
	return false
}
