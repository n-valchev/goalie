package config

import (
	"fmt"
	"strconv"
)

// Rules has one field for each rule, so that the strict decoder in Read rejects an unknown rule name.
type Rules struct {
	Dup DupRule `json:"dup"`
}

// Control holds what every rule has. An empty value means that the file leaves it out, because the
// default mode differs from rule to rule.
type Control struct {
	Mode  Mode  `json:"mode"`
	Scope Scope `json:"scope"`
}

// DupRule holds pointers for the same reason as Godry.
type DupRule struct {
	Control
	Threshold    *float64 `json:"threshold"`
	MinLines     *int     `json:"minLines"`
	MinNodes     *int     `json:"minNodes"`
	WeakAnchors  *int     `json:"weakAnchors"`
	WeakLiterals *float64 `json:"weakLiterals"`
	IncludeTests *bool    `json:"includeTests"`
}

type Mode string

const (
	ModeOff  Mode = "off"
	ModeWarn Mode = "warn"
	ModeFail Mode = "fail"
)

// UnmarshalJSON checks the value here, so that a misspelled mode fails in every tool.
func (m *Mode) UnmarshalJSON(data []byte) error {
	return decodeChoice(m, data, "mode", ModeOff, ModeWarn, ModeFail)
}

type Scope string

const (
	ScopeChanges Scope = "changes"
	ScopeAll     Scope = "all"
)

// UnmarshalJSON checks the value here, for the same reason as Mode.
func (s *Scope) UnmarshalJSON(data []byte) error {
	return decodeChoice(s, data, "scope", ScopeChanges, ScopeAll)
}

func decodeChoice[T ~string](target *T, data []byte, name string, choices ...T) error {
	if string(data) == "null" {
		return nil
	}
	for _, choice := range choices {
		if string(data) == strconv.Quote(string(choice)) {
			*target = choice
			return nil
		}
	}
	return fmt.Errorf("invalid %s %s: must be one of %v", name, data, choices)
}
