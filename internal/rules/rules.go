// Package rules holds the measures of the function rules. It works on parsed Go code and knows
// nothing about Git, settings, or output, which internal/check adds once for every rule.
package rules

import "github.com/n-valchev/goalie/internal/extract"

// Lines counts blank lines and comments too, because an agent reads them all.
func Lines(fn extract.Function) int {
	return fn.Lines()
}
