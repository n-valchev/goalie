// Package cliflags holds the flag handling that all goalie tools share.
package cliflags

import (
	"errors"
	"flag"
)

// Parsed is what the command line held, apart from the flag values themselves.
type Parsed struct {
	Paths []string
	Help  bool
	Set   map[string]bool // A flag's value alone can't show this, because a flag set to its default must still win.
}

// Parse accepts flags after paths. The flag package stops at the first path, which would silently
// turn a later flag into a path.
func Parse(flags *flag.FlagSet, args []string) (Parsed, error) {
	var parsed Parsed
	for rest := args; ; {
		err := flags.Parse(rest)
		if errors.Is(err, flag.ErrHelp) {
			parsed.Help = true
			return parsed, nil
		}
		if err != nil {
			return parsed, err
		}
		remaining := flags.Args()
		consumed := rest[:len(rest)-len(remaining)]
		if len(consumed) > 0 && consumed[len(consumed)-1] == "--" {
			parsed.Paths = append(parsed.Paths, remaining...)
			break
		}
		if len(remaining) == 0 {
			break
		}
		parsed.Paths = append(parsed.Paths, remaining[0])
		rest = remaining[1:]
	}
	parsed.Set = map[string]bool{}
	flags.Visit(func(set *flag.Flag) { parsed.Set[set.Name] = true })
	return parsed, nil
}

// Fill copies a value from the settings file, unless the flag was set.
func Fill[T any](set map[string]bool, flagName string, target, value *T) {
	if value != nil && !set[flagName] {
		*target = *value
	}
}
