package bootstrap

import (
	"errors"
	"reflect"
	"testing"
)

// Parser tests for the commander.js semantics reimplemented in mcp_parse.go.
// Error strings are byte-pinned against oracle captures.

func parseOK(t *testing.T, args []string, specs []mcpFlagSpec) *mcpParsed {
	t.Helper()
	p, err := parseMCPFlags(args, specs)
	if err != nil {
		t.Fatalf("parseMCPFlags(%q): %v", args, err)
	}
	return p
}

func TestParseMCPFlagsValueForms(t *testing.T) {
	t.Run("long-separate", func(t *testing.T) {
		p := parseOK(t, []string{"--scope", "user", "n"}, mcpAddFlagSpecs)
		if p.Values["scope"] != "user" || len(p.Positionals) != 1 || p.Positionals[0] != "n" {
			t.Fatalf("p = %+v", p)
		}
	})
	t.Run("long-inline", func(t *testing.T) {
		p := parseOK(t, []string{"--scope=user"}, mcpAddFlagSpecs)
		if p.Values["scope"] != "user" {
			t.Fatalf("p = %+v", p)
		}
	})
	t.Run("short-cluster", func(t *testing.T) {
		p := parseOK(t, []string{"-slocal"}, mcpAddFlagSpecs)
		if p.Values["scope"] != "local" {
			t.Fatalf("p = %+v", p)
		}
	})
	t.Run("short-separate", func(t *testing.T) {
		p := parseOK(t, []string{"-s", "local"}, mcpAddFlagSpecs)
		if p.Values["scope"] != "local" {
			t.Fatalf("p = %+v", p)
		}
	})
	t.Run("short-cluster-value-with-eq", func(t *testing.T) {
		p := parseOK(t, []string{"-e=K=V"}, mcpAddFlagSpecs)
		if !reflect.DeepEqual(p.Lists["env"], []string{"K=V"}) {
			t.Fatalf("lists = %+v", p.Lists)
		}
	})
	t.Run("short-bool-cluster", func(t *testing.T) {
		// -dh is a legal cluster: -d bool then -h bool (help wins).
		p, err := parseMCPFlags([]string{"-dh"}, mcpServeFlagSpecs)
		if err != nil {
			t.Fatal(err)
		}
		if !p.SawHelp || !p.Bools["debug"] {
			t.Fatalf("p = %+v", p)
		}
	})
	t.Run("last-value-wins", func(t *testing.T) {
		p := parseOK(t, []string{"-s", "user", "-s", "local"}, mcpAddFlagSpecs)
		if p.Values["scope"] != "local" {
			t.Fatalf("p = %+v", p)
		}
	})
}

func TestParseMCPFlagsHelpWins(t *testing.T) {
	// -h mid-walk returns immediately; a later unknown option must not error.
	p := parseOK(t, []string{"x", "-h", "--nope"}, mcpAddFlagSpecs)
	if !p.SawHelp {
		t.Fatalf("p = %+v", p)
	}
	// Long form too.
	p = parseOK(t, []string{"--help", "-z"}, mcpParentFlagSpecs)
	if !p.SawHelp {
		t.Fatalf("p = %+v", p)
	}
}

func TestParseMCPFlagsUnknownOption(t *testing.T) {
	t.Run("long-with-suggestion", func(t *testing.T) {
		_, err := parseMCPFlags([]string{"add", "--nope", "x", "y"}, mcpAddFlagSpecs)
		var e *mcpUsageError
		if !errors.As(err, &e) {
			t.Fatalf("err = %v (%T)", err, err)
		}
		if e.Line1 != "error: unknown option '--nope'" || e.Line2 != "(Did you mean --scope?)" {
			t.Fatalf("error = %q / %q", e.Line1, e.Line2)
		}
	})
	t.Run("long-inline-reports-whole-token", func(t *testing.T) {
		_, err := parseMCPFlags([]string{"--nope=1"}, mcpAddFlagSpecs)
		var e *mcpUsageError
		if !errors.As(err, &e) || e.Line1 != "error: unknown option '--nope=1'" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("short-no-suggestion", func(t *testing.T) {
		_, err := parseMCPFlags([]string{"-z"}, mcpAddFlagSpecs)
		var e *mcpUsageError
		if !errors.As(err, &e) {
			t.Fatalf("err = %v", err)
		}
		if e.Line1 != "error: unknown option '-z'" || e.Line2 != "" {
			t.Fatalf("error = %q / %q", e.Line1, e.Line2)
		}
	})
	t.Run("short-in-cluster", func(t *testing.T) {
		// An unknown short after a bool one reports just the offending char,
		// not the whole cluster (oracle: `mcp serve -dz`).
		_, err := parseMCPFlags([]string{"-dz"}, mcpServeFlagSpecs)
		var e *mcpUsageError
		if !errors.As(err, &e) || e.Line1 != "error: unknown option '-z'" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("value-short-eats-cluster-rest", func(t *testing.T) {
		// -sz is --scope z (the rest of the cluster is the value), not an
		// unknown option (oracle: `mcp add x y -sz` → "Invalid scope: z").
		p := parseOK(t, []string{"x", "y", "-sz"}, mcpAddFlagSpecs)
		if p.Values["scope"] != "z" {
			t.Fatalf("scope = %q", p.Values["scope"])
		}
	})
}

func TestParseMCPFlagsMissingArgument(t *testing.T) {
	t.Run("value-flag-at-end", func(t *testing.T) {
		_, err := parseMCPFlags([]string{"x", "-s"}, mcpAddFlagSpecs)
		var e *mcpUsageError
		if !errors.As(err, &e) || e.Line1 != "error: option '-s, --scope <scope>' argument missing" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("greedy-flag-at-end", func(t *testing.T) {
		_, err := parseMCPFlags([]string{"n", "-e"}, mcpAddFlagSpecs)
		var e *mcpUsageError
		if !errors.As(err, &e) || e.Line1 != "error: option '-e, --env <env...>' argument missing" {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestParseMCPFlagsGreedy(t *testing.T) {
	t.Run("consumes-until-dash", func(t *testing.T) {
		p := parseOK(t, []string{"beta", "-e", "K1=v1", "-e", "K2=v2", "--", "cat", "-A"}, mcpAddFlagSpecs)
		if !reflect.DeepEqual(p.Lists["env"], []string{"K1=v1", "K2=v2"}) {
			t.Fatalf("env = %+v", p.Lists["env"])
		}
		if !reflect.DeepEqual(p.Positionals, []string{"beta", "cat", "-A"}) {
			t.Fatalf("positionals = %+v", p.Positionals)
		}
	})
	t.Run("inline-single-value", func(t *testing.T) {
		// --env=K=V supplies exactly one value — no greedy continuation, so a
		// following bare token stays positional.
		p := parseOK(t, []string{"n", "--env=K=V", "cmd"}, mcpAddFlagSpecs)
		if !reflect.DeepEqual(p.Lists["env"], []string{"K=V"}) {
			t.Fatalf("env = %+v", p.Lists["env"])
		}
		if !reflect.DeepEqual(p.Positionals, []string{"n", "cmd"}) {
			t.Fatalf("positionals = %+v", p.Positionals)
		}
	})
	t.Run("swallows-url", func(t *testing.T) {
		// The oracle-captured greedy trap: after -H, the URL is eaten.
		p := parseOK(t, []string{"hh2", "-t", "http", "-H", "X-Api:abc", "-H", "X-Custom:val", "http://hh2.test/mcp"}, mcpAddFlagSpecs)
		if !reflect.DeepEqual(p.Positionals, []string{"hh2"}) {
			t.Fatalf("positionals = %+v", p.Positionals)
		}
		if len(p.Lists["header"]) != 3 {
			t.Fatalf("header = %+v", p.Lists["header"])
		}
	})
}

func TestParseMCPFlagsDoubleDash(t *testing.T) {
	// After --, every token is positional — even ones that look like flags.
	p := parseOK(t, []string{"--", "-h", "add"}, mcpParentFlagSpecs)
	if p.SawHelp {
		t.Fatal("post-`--` tokens must not trigger help")
	}
	if !reflect.DeepEqual(p.Positionals, []string{"-h", "add"}) {
		t.Fatalf("positionals = %+v", p.Positionals)
	}
	// A dash-prefixed positional before -- is still a positional.
	p = parseOK(t, []string{"get", "--", "-x"}, mcpGetFlagSpecs)
	if !reflect.DeepEqual(p.Positionals, []string{"get", "-x"}) {
		t.Fatalf("positionals = %+v", p.Positionals)
	}
}

func TestMCPCheckPositionals(t *testing.T) {
	cases := []struct {
		positionals []string
		names       string
		wantMiss    string // "" for ok
	}{
		{nil, mcpAddPositionals, "name"},
		{[]string{"x"}, mcpAddPositionals, "commandOrUrl"},
		{[]string{"x", "y"}, mcpAddPositionals, ""},
		{[]string{"x", "y", "z"}, mcpAddPositionals, ""},
		{nil, mcpNamePositional, "name"},
		{[]string{"x"}, mcpNamePositional, ""},
		{nil, "", ""},
	}
	for _, tc := range cases {
		p := &mcpParsed{Positionals: tc.positionals}
		err := mcpCheckPositionals(p, tc.names)
		if tc.wantMiss == "" {
			if err != nil {
				t.Fatalf("positionals=%v names=%q: unexpected error %v", tc.positionals, tc.names, err)
			}
			continue
		}
		var missing *mcpMissingArgError
		if !errors.As(err, &missing) || missing.Name != tc.wantMiss {
			t.Fatalf("positionals=%v names=%q: err = %v, want missing %q", tc.positionals, tc.names, err, tc.wantMiss)
		}
	}
}

func TestDamerauLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"abc", "abc", 0},
		{"alpga", "alpha", 1}, // adjacent transposition
		{"lst", "list", 1},
		{"kitten", "sitting", 3},
		{"ca", "abc", 3}, // OSA: no substring move
	}
	for _, tc := range cases {
		if got := damerauLevenshtein(tc.a, tc.b); got != tc.want {
			t.Fatalf("d(%q,%q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSuggestSimilar(t *testing.T) {
	t.Run("commander-commands", func(t *testing.T) {
		// Oracle captures: `mcp lst` → list; `mcp server` → serve.
		if got := suggestSimilar("lst", mcpCommandNames); !reflect.DeepEqual(got, []string{"list"}) {
			t.Fatalf("lst → %v", got)
		}
		if got := suggestSimilar("server", mcpCommandNames); !reflect.DeepEqual(got, []string{"serve"}) {
			t.Fatalf("server → %v", got)
		}
	})
	t.Run("flag-suggestion", func(t *testing.T) {
		got := suggestSimilar("--nope", mcpLongFlagNames(mcpAddFlagSpecs))
		if !reflect.DeepEqual(got, []string{"--scope"}) {
			t.Fatalf("--nope → %v", got)
		}
	})
	t.Run("no-candidates", func(t *testing.T) {
		if got := suggestSimilar("x", nil); got != nil {
			t.Fatalf("x → %v", got)
		}
		if got := suggestSimilar("", mcpCommandNames); got != nil {
			t.Fatalf("empty → %v", got)
		}
	})
	t.Run("best-distance-only", func(t *testing.T) {
		// "get" is within the threshold of "lst" (d=3, sim=0.5) but only the
		// best distance is kept.
		got := suggestSimilar("lst", []string{"get", "list"})
		if !reflect.DeepEqual(got, []string{"list"}) {
			t.Fatalf("lst → %v", got)
		}
	})
	t.Run("ties-sorted", func(t *testing.T) {
		got := suggestSimilar("abcd", []string{"abce", "abcf", "zzzz"})
		if !reflect.DeepEqual(got, []string{"abce", "abcf"}) {
			t.Fatalf("abcd → %v", got)
		}
	})
}

func TestSuggestServerName(t *testing.T) {
	sorted := []string{"alpha", "beta", "gamma"}
	if got := suggestServerName("alpga", sorted); got != "alpha" {
		t.Fatalf("alpga → %q", got)
	}
	if got := suggestServerName("ghost", sorted); got != "" {
		t.Fatalf("ghost → %q", got)
	}
	// The oracle suggests up to edit distance 2 (probe: get alphaxx → alpha).
	if got := suggestServerName("alphaxx", []string{"alpha"}); got != "alpha" {
		t.Fatalf("alphaxx → %q, want \"alpha\" (distance 2 is at the threshold)", got)
	}
	// Distance 3 is beyond it (probe: get alphaxxx → no suggestion).
	if got := suggestServerName("alphaxxx", []string{"alpha"}); got != "" {
		t.Fatalf("alphaxxx → %q, want \"\" (distance 3 exceeds the threshold)", got)
	}
	// First candidate wins ties (candidates must be sorted).
	if got := suggestServerName("abf", []string{"abc", "abd"}); got != "abc" {
		t.Fatalf("abf → %q", got)
	}
}
