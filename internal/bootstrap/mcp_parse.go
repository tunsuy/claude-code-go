package bootstrap

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// This file re-implements the option-parsing semantics of the upstream TS CLI
// (commander.js) for the `claude mcp` tree.  Every subcommand disables cobra's
// flag parsing and receives the raw argv tail instead; parseMCPFlags then
// reproduces commander's behavior byte-for-byte:
//
//   - `-h/--help` encountered anywhere during the walk wins immediately (help
//     is shown even if a later token would have been an unknown option).
//   - Unknown options error on the raw token (`--nope=val` reports the whole
//     token); long options get a `(Did you mean …?)` suggestion, shorts never.
//   - `-e/--env` and `-H/--header` are greedy: each occurrence consumes all
//     following non-dash tokens (this is why `add n -t http -H A:1 URL` fails
//     with a missing commandOrUrl — the URL is swallowed as a header value).
//   - `--` switches to positionals-only mode.
//
// All error strings below were captured from the oracle (claude v2.1.261).

// mcpFlagKind describes how a flag consumes values.
type mcpFlagKind int

const (
	mcpFlagValue  mcpFlagKind = iota // -s, --scope <scope>
	mcpFlagBool                      // --client-secret
	mcpFlagGreedy                    // -e, --env <env...>
)

// mcpFlagSpec is one option accepted by an mcp subcommand.
type mcpFlagSpec struct {
	Long  string // canonical long name, without dashes
	Short string // short name, without dash ("" when none)
	Kind  mcpFlagKind
	Flags string // display form used by "argument missing" errors, e.g. "-s, --scope <scope>"
}

// mcpParsed is the result of a commander-style parse.
type mcpParsed struct {
	Positionals []string
	Values      map[string]string   // last value of each value-kind flag
	Lists       map[string][]string // collected values of greedy flags
	Bools       map[string]bool
	SawHelp     bool
}

// mcpUsageError is a commander-style usage error: "error: …" plus an optional
// "(Did you mean …?)" second line.
type mcpUsageError struct {
	Line1 string
	Line2 string // empty when there is no suggestion
}

func (e *mcpUsageError) Error() string {
	if e.Line2 == "" {
		return e.Line1
	}
	return e.Line1 + "\n" + e.Line2
}

// mcpMissingArgError is the commander "missing required argument" error.
type mcpMissingArgError struct {
	Name string
}

func (e *mcpMissingArgError) Error() string {
	return fmt.Sprintf("error: missing required argument '%s'", e.Name)
}

// Flag specs for each subcommand (help is always present).

var (
	mcpScopeFlag = mcpFlagSpec{Long: "scope", Short: "s", Kind: mcpFlagValue, Flags: "-s, --scope <scope>"}

	mcpHelpFlag = mcpFlagSpec{Long: "help", Short: "h", Kind: mcpFlagBool, Flags: "-h, --help"}

	mcpAddFlagSpecs = []mcpFlagSpec{
		{Long: "callback-port", Kind: mcpFlagValue, Flags: "--callback-port <port>"},
		{Long: "client-id", Kind: mcpFlagValue, Flags: "--client-id <clientId>"},
		{Long: "client-secret", Kind: mcpFlagBool, Flags: "--client-secret"},
		{Long: "env", Short: "e", Kind: mcpFlagGreedy, Flags: "-e, --env <env...>"},
		{Long: "header", Short: "H", Kind: mcpFlagGreedy, Flags: "-H, --header <header...>"},
		mcpHelpFlag,
		mcpScopeFlag,
		{Long: "transport", Short: "t", Kind: mcpFlagValue, Flags: "-t, --transport <transport>"},
	}

	mcpRemoveFlagSpecs = []mcpFlagSpec{mcpHelpFlag, mcpScopeFlag}

	mcpListFlagSpecs = []mcpFlagSpec{mcpHelpFlag}

	mcpGetFlagSpecs = []mcpFlagSpec{mcpHelpFlag}

	mcpAddJSONFlagSpecs = []mcpFlagSpec{
		{Long: "client-secret", Kind: mcpFlagBool, Flags: "--client-secret"},
		mcpHelpFlag,
		mcpScopeFlag,
	}

	mcpAfcdFlagSpecs = []mcpFlagSpec{mcpHelpFlag, mcpScopeFlag}

	mcpResetFlagSpecs = []mcpFlagSpec{mcpHelpFlag}

	mcpServeFlagSpecs = []mcpFlagSpec{
		{Long: "debug", Short: "d", Kind: mcpFlagBool, Flags: "-d, --debug"},
		mcpHelpFlag,
		{Long: "verbose", Kind: mcpFlagBool, Flags: "--verbose"},
	}

	mcpParentFlagSpecs = []mcpFlagSpec{mcpHelpFlag}
)

// Required positional argument names per subcommand, in declaration order.
const (
	mcpAddPositionals     = "name,commandOrUrl"
	mcpNamePositional     = "name"
	mcpAddJSONPositionals = "name,json"
)

// parseMCPFlags walks args with commander semantics.  specs must contain the
// help flag.  On success the caller checks SawHelp first: when set, parsing
// stopped at -h/--help and no further validation applies.
func parseMCPFlags(args []string, specs []mcpFlagSpec) (*mcpParsed, error) {
	byLong := make(map[string]*mcpFlagSpec, len(specs))
	byShort := make(map[string]*mcpFlagSpec, len(specs))
	for i := range specs {
		byLong[specs[i].Long] = &specs[i]
		if specs[i].Short != "" {
			byShort[specs[i].Short] = &specs[i]
		}
	}

	p := &mcpParsed{
		Values: map[string]string{},
		Lists:  map[string][]string{},
		Bools:  map[string]bool{},
	}
	sawDD := false
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if sawDD {
			p.Positionals = append(p.Positionals, tok)
			continue
		}
		if tok == "--" {
			sawDD = true
			continue
		}
		if strings.HasPrefix(tok, "--") {
			body := tok[2:]
			name, val := body, ""
			hasVal := false
			if idx := strings.Index(body, "="); idx >= 0 {
				name, val, hasVal = body[:idx], body[idx+1:], true
			}
			spec, ok := byLong[name]
			if !ok {
				return nil, mcpUnknownOptionError(tok, mcpLongFlagNames(specs))
			}
			if err := p.applyFlag(spec, name, val, hasVal, args, &i); err != nil {
				return nil, err
			}
			if p.SawHelp {
				// Commander's help handler fires mid-walk: a later unknown
				// option must not turn into an error.
				return p, nil
			}
			continue
		}
		if strings.HasPrefix(tok, "-") && len(tok) > 1 {
			// Short option cluster: "-slocal", "-eK=V", "-dh" are all legal.
			body := tok[1:]
			for len(body) > 0 {
				ch := body[:1]
				spec, ok := byShort[ch]
				if !ok {
					// Commander reports just the offending short char, never
					// the whole cluster token (oracle: `mcp serve -dz` →
					// "error: unknown option '-z'").
					return nil, mcpUnknownOptionError("-"+ch, nil)
				}
				rest := body[1:]
				if spec.Kind == mcpFlagBool {
					p.Bools[spec.Long] = true
					if spec.Long == "help" {
						p.SawHelp = true
						return p, nil
					}
					body = rest
					continue
				}
				// Value/greedy short: remainder of the token is the value.
				rest = strings.TrimPrefix(rest, "=")
				if rest != "" {
					if err := p.applyFlag(spec, spec.Long, rest, true, args, &i); err != nil {
						return nil, err
					}
				} else if err := p.applyFlag(spec, spec.Long, "", false, args, &i); err != nil {
					return nil, err
				}
				body = ""
			}
			continue
		}
		p.Positionals = append(p.Positionals, tok)
	}
	return p, nil
}

// applyFlag records one option occurrence.  For value flags without an inline
// value it consumes the next arg; for greedy flags it consumes all following
// non-dash args.
func (p *mcpParsed) applyFlag(spec *mcpFlagSpec, name, val string, hasVal bool, args []string, i *int) error {
	switch spec.Kind {
	case mcpFlagBool:
		p.Bools[spec.Long] = true
		if spec.Long == "help" {
			p.SawHelp = true
		}
	case mcpFlagValue:
		if hasVal {
			p.Values[spec.Long] = val
			return nil
		}
		if *i+1 >= len(args) {
			return &mcpUsageError{Line1: fmt.Sprintf("error: option '%s' argument missing", spec.Flags)}
		}
		*i++
		p.Values[spec.Long] = args[*i]
	case mcpFlagGreedy:
		if hasVal {
			// "--env=K=V" supplies exactly one value, no greedy continuation.
			p.Lists[spec.Long] = append(p.Lists[spec.Long], val)
			return nil
		}
		consumed := false
		for *i+1 < len(args) && !strings.HasPrefix(args[*i+1], "-") {
			*i++
			p.Lists[spec.Long] = append(p.Lists[spec.Long], args[*i])
			consumed = true
		}
		if !consumed {
			return &mcpUsageError{Line1: fmt.Sprintf("error: option '%s' argument missing", spec.Flags)}
		}
	}
	return nil
}

// mcpUnknownOptionError builds the unknown-option error for tok.  Suggestions
// are only computed for long options, over the command's long flag names.
func mcpUnknownOptionError(tok string, longCandidates []string) error {
	e := &mcpUsageError{Line1: fmt.Sprintf("error: unknown option '%s'", tok)}
	if strings.HasPrefix(tok, "--") {
		if s := suggestSimilar(tok, longCandidates); len(s) > 0 {
			e.Line2 = "(Did you mean " + strings.Join(s, " or ") + "?)"
		}
	}
	return e
}

// mcpLongFlagNames lists a command's long flag names with dashes, for
// suggestions.
func mcpLongFlagNames(specs []mcpFlagSpec) []string {
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, "--"+s.Long)
	}
	return names
}

// mcpCheckPositionals returns a missing-argument error when fewer positionals
// were supplied than the command requires.  names is a comma-separated list in
// declaration order.
func mcpCheckPositionals(p *mcpParsed, names string) error {
	if names == "" {
		return nil
	}
	required := strings.Split(names, ",")
	if len(p.Positionals) < len(required) {
		return &mcpMissingArgError{Name: required[len(p.Positionals)]}
	}
	return nil
}

// damerauLevenshtein computes the optimal-string-alignment edit distance
// (Levenshtein with adjacent transpositions), the metric commander and the
// oracle's server-name suggestions are built on.
func damerauLevenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev2 := make([]int, lb+1)
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			m := prev[j] + 1              // deletion
			if v := cur[j-1] + 1; v < m { // insertion
				m = v
			}
			if v := prev[j-1] + cost; v < m { // substitution
				m = v
			}
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				if v := prev2[j-2] + 1; v < m { // transposition
					m = v
				}
			}
			cur[j] = m
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[lb]
}

// suggestSimilar mirrors commander's suggestSimilar: candidates within
// damerau distance 3 whose similarity ((maxLen-dist)/maxLen) exceeds 0.4,
// keeping all candidates at the best distance, sorted alphabetically.
func suggestSimilar(word string, candidates []string) []string {
	if word == "" || len(candidates) == 0 {
		return nil
	}
	best := math.MaxInt
	var out []string
	for _, c := range candidates {
		if len(c) <= 1 {
			continue
		}
		d := damerauLevenshtein(word, c)
		maxLen := len(word)
		if len(c) > maxLen {
			maxLen = len(c)
		}
		sim := float64(maxLen-d) / float64(maxLen)
		if d > 3 || sim <= 0.4 {
			continue
		}
		if d < best {
			best = d
			out = out[:0]
		}
		if d == best {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// suggestServerName picks the closest configured server name for a miss
// message, mirroring the oracle's suggester: candidates within OSA-damerau
// distance 2 (its length-difference gate is a pure fast-path — distance is
// always ≥ the length difference), first candidate on ties, so candidates
// must be sorted.
func suggestServerName(input string, candidates []string) string {
	best := 3
	bestName := ""
	for _, c := range candidates {
		diff := len(input) - len(c)
		if diff < 0 {
			diff = -diff
		}
		if diff > 2 {
			continue
		}
		d := damerauLevenshtein(input, c)
		if d < best {
			best = d
			bestName = c
		}
	}
	return bestName
}
