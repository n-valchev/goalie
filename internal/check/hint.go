package check

// The hints are the fix list of the README section "Steer coding agents", and a test keeps the two
// the same.
var hints = map[string]string{
	DupRule:                  "Reuse the other function, or move the shared code to one place. Don't keep the copy.",
	CrapRule:                 "Split the function. No test can bring it under the limit.",
	LongFunction.Name:        "Split the function into smaller functions with clear names.",
	ManyArguments.Name:       groupParametersHint,
	ManyDependencies.Name:    groupParametersHint,
	ComplexCondition.Name:    "Give parts of the condition names, such as `ready := a && b`, or move the condition into a function.",
	CognitiveComplexity.Name: "Return early in place of a nested `if`, or move a nested block into its own function.",
	PrimitiveArguments.Name:  "Define a type for the value, such as `type UserID string`, and use it in each function that passes the value down.",
	LongFile.Name:            "Move a group of functions that belong together into a file of their own.",
	HighMeanComplexity.Name:  "Split the most complex functions, or move them into a file of their own.",
	LowCohesion.Name:         "Move each group of methods, with the fields that it uses, into a type of its own.",
}

const groupParametersHint = "Group parameters that belong together into a type, or split the function."

const needsCoverageHint = "Write tests that cover at least that much of the function."

// AddHints is for the agent of goalie serve, which reads one result at a time and has no README.
func AddHints(results []Result) {
	for i, result := range results {
		results[i].Hint = hints[result.Rule]
		if estimate, ok := result.Details.(Estimate); ok && estimate.CanPass {
			results[i].Hint = needsCoverageHint
		}
	}
}
