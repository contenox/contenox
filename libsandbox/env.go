package libsandbox

import (
	"sort"
	"strings"
)

// scrubEnv builds the minimal environment the confined process is allowed to
// inherit — the core of the credential-leak fix. The starting point is nothing:
// a bare exec.Command that appends os.Environ() hands the child every secret in
// the parent (AWS keys, tokens, npm creds), which is exactly what a compromised
// postinstall script exfiltrates. Instead only the names in allow (matched
// exactly) are copied out of parentEnv, then set is layered on for explicit
// extras, and finally HOME is forced to home.
//
// Precedence, last wins: an allow-copied value < a set value < HOME=home. So a
// caller can pass a variable through by name yet override it explicitly, but the
// scoped HOME is authoritative over both — it is the mechanism that keeps
// ~/.ssh, ~/.aws, and ~/.contenox out of reach, not a default to be overridden.
//
// The result is sorted by name, so the same inputs always produce the same
// slice: the function is pure and deterministic, which keeps it testable and
// keeps the emitted environment stable across runs. Malformed parent entries
// (no "=") are skipped — there is no name to match them by. home is used as
// given; validating that it is non-empty is Command's job.
func scrubEnv(parentEnv []string, allow []string, set map[string]string, home string) []string {
	allowed := make(map[string]struct{}, len(allow))
	for _, name := range allow {
		allowed[name] = struct{}{}
	}

	out := make(map[string]string, len(allow)+len(set)+1)
	for _, kv := range parentEnv {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue // no name to match; not a real KEY=VALUE entry
		}
		name := kv[:eq]
		if _, ok := allowed[name]; ok {
			out[name] = kv[eq+1:]
		}
	}
	for k, v := range set {
		out[k] = v
	}
	// HOME is forced to the scoped home dir, overriding anything inherited or
	// set: the scoped HOME is the whole reason ~/.ssh, ~/.aws, ~/.npmrc, and
	// ~/.contenox are not reachable (see Spec.Home).
	out["HOME"] = home

	names := make([]string, 0, len(out))
	for name := range out {
		names = append(names, name)
	}
	sort.Strings(names)

	env := make([]string, 0, len(names))
	for _, name := range names {
		env = append(env, name+"="+out[name])
	}
	return env
}
