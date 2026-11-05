package compare

import (
	"strings"

	"github.com/n-valchev/goalie/internal/files"
)

// Case says where the two functions of a pair are, because the right fix depends on it.
type Case string

const (
	CopiesShared   Case = "copies-shared"
	AcrossProducts Case = "across-products"
	SameProduct    Case = "same-product"
	WithinShared   Case = "within-shared"
)

// Classify sets the case of each pair, and of none when there are no shared patterns: without them,
// a shared package would look like a product and the case would mislead.
func Classify(pairs []Pair, shared []string) {
	if len(shared) == 0 {
		return
	}
	for i, pair := range pairs {
		pairs[i].Case = caseOf(pair.Left.File, pair.Right.File, shared)
	}
}

func caseOf(left, right string, shared []string) Case {
	leftShared, rightShared := files.MatchesAny(shared, left), files.MatchesAny(shared, right)
	leftProduct, rightProduct := product(left), product(right)
	switch {
	case leftShared && rightShared:
		return WithinShared
	case leftShared && rightProduct != "", rightShared && leftProduct != "":
		return CopiesShared
	case leftShared, rightShared, leftProduct == "", rightProduct == "":
		return ""
	case leftProduct == rightProduct:
		return SameProduct
	}
	return AcrossProducts
}

// product is the first folder of a path from the run directory. Godry takes no list of products,
// so that it works with any repo layout. A file above the run directory has no first folder to use.
func product(file string) string {
	first, _, nested := strings.Cut(file, "/")
	if !nested || first == ".." {
		return ""
	}
	return first
}
