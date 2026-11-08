package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Rules has one field for each rule, so that the strict decoder in Read rejects an unknown rule name.
type Rules struct {
	Dup                 DupRule      `json:"dup"`
	Crap                CrapRule     `json:"crap"`
	LongFunction        FunctionRule `json:"long-function"`
	ManyArguments       FunctionRule `json:"many-arguments"`
	ManyDependencies    FunctionRule `json:"many-dependencies"`
	ComplexCondition    FunctionRule `json:"complex-condition"`
	CognitiveComplexity FunctionRule `json:"cognitive-complexity"`
	PrimitiveArguments  FunctionRule `json:"primitive-arguments"`
}

// Control holds what every rule has. An empty value means that the file leaves it out.
type Control struct {
	Mode  Mode  `json:"mode"`
	Scope Scope `json:"scope"`
}

// DupRule holds pointers for the same reason as Godeps.
type DupRule struct {
	Control
	Threshold    *float64 `json:"threshold"`
	MinLines     *int     `json:"minLines"`
	MinNodes     *int     `json:"minNodes"`
	WeakAnchors  *int     `json:"weakAnchors"`
	WeakLiterals *float64 `json:"weakLiterals"`
	IncludeTests *bool    `json:"includeTests"`
}

// CrapRule holds pointers for the same reason as Godeps.
type CrapRule struct {
	Control
	Cover
	Limit       *float64 `json:"limit"`
	SharedLimit *float64 `json:"sharedLimit"`
}

// FunctionRule is the settings of a rule that measures one number for each function. It holds a
// pointer for the same reason as Godeps.
type FunctionRule struct {
	Control
	Limit *float64 `json:"limit"`
}

type Cover struct {
	CoverCommand  *string  `json:"coverCommand"`
	CoverProfiles []string `json:"coverProfiles"`
}

func (c Cover) Check() error {
	if c.CoverCommand != nil && strings.TrimSpace(*c.CoverCommand) == "" {
		return errors.New("coverCommand must not be empty")
	}
	// The file can't say which one wins, so a flag does not excuse this either.
	if c.CoverCommand != nil && len(c.CoverProfiles) > 0 {
		return errors.New("set coverCommand or coverProfiles, not both")
	}
	return nil
}

type Mode string

const (
	ModeOff  Mode = "off"
	ModeWarn Mode = "warn"
	ModeFail Mode = "fail"
)

// UnmarshalJSON checks the value here, so that a misspelled mode fails in every command that reads the file.
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
