package model

import (
	"cmp"
	"slices"
	"strings"
)

// sharedBox holds all shared classes. An import path can't hold a colon, so the id never clashes
// with a folder.
const sharedBox = ":shared"

// Package is a folder box that groups classes and other boxes.
type Package struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Parent         string `json:"parent,omitempty"`
	Shared         bool   `json:"shared,omitempty"`
	PackageCount   int    `json:"packageCount"`
	DuplicateCount *int   `json:"duplicateCount,omitempty"`
	Grade          Grade  `json:"grade,omitempty"`
}

func boxesByID(result *Model) map[string]*Package {
	boxes := map[string]*Package{}
	for i := range result.Packages {
		boxes[result.Packages[i].ID] = &result.Packages[i]
	}
	return boxes
}

func placeClasses(classes []Class, shared map[string]bool) []Package {
	boxes := map[string]*Package{}
	for i := range classes {
		class := &classes[i]
		enter := func(id, name string) {
			box := boxes[id]
			if box == nil {
				box = &Package{ID: id, Name: name, Parent: class.Package, Shared: shared[class.ID]}
				boxes[id] = box
			}
			box.PackageCount++
			class.Package = id
		}
		prefix := ""
		if shared[class.ID] {
			enter(sharedBox, "shared")
			prefix = sharedBox + "/"
		}
		if class.Path == "." {
			continue
		}
		folders := strings.Split(class.Path, "/")
		for depth, name := range folders {
			enter(prefix+strings.Join(folders[:depth+1], "/"), name)
		}
	}
	result := make([]Package, 0, len(boxes))
	for _, box := range boxes {
		result = append(result, *box)
	}
	slices.SortFunc(result, func(a, b Package) int { return cmp.Compare(a.ID, b.ID) })
	return result
}
