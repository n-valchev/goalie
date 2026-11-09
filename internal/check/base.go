package check

import (
	"maps"
	"path"
	"slices"
)

// Place names the code that a result is about: a function or a type by name, or a whole file with
// no name. A renamed function is a new place.
type Place struct {
	File string
	Name string
}

// Change says what the change did to a result, so that an agent can tell its own results from old ones.
type Change string

const (
	ChangeNew       Change = "new"
	ChangeWorse     Change = "worse"
	ChangeUnchanged Change = "unchanged"
)

// AtBase returns a run of the code at the merge base of rev and HEAD: each changed file holds its
// content there, and a file that is new since then is gone. It lists only the folders of results,
// because a result can match only a base result about the same places, and a run of every file
// would double the time of a check.
func (s Scope) AtBase(dir, rev string, results []Result) (Scope, error) {
	folders := map[string]bool{}
	for _, result := range results {
		for _, place := range result.About {
			folders[path.Dir(place.File)] = true
		}
	}
	inFolders := func(file string) bool { return folders[path.Dir(file)] }
	// With the tests, because dup may read them.
	paths, err := s.Files.find(s.Repo, dir, true)
	if err != nil {
		return Scope{}, err
	}
	changes, err := s.Changes.Since(dir, rev)
	if err != nil {
		return Scope{}, err
	}
	changed := slices.Collect(maps.Keys(changes.For(dir, slices.DeleteFunc(paths, func(file string) bool { return !inFolders(file) }))))
	bases, err := s.Changes.Base(dir, rev, changed)
	if err != nil {
		return Scope{}, err
	}
	sources, gone := map[string][]byte{}, map[string]bool{}
	for _, file := range changed {
		if base, ok := bases[file]; ok && !base.New {
			sources[file] = base.Content
		} else {
			gone[file] = true
		}
	}
	return Scope{
		Repo: s.Repo, Parser: s.Parser.Replace(sources), Changes: s.Changes, Warnings: NewWarnings(),
		Files: &Files{from: s.Files, keep: func(file string) bool { return inFolders(file) && !gone[file] }},
	}, nil
}

// Mark sets Change on each of results from base, the results that the same rules gave at the base.
// Two init functions of one file share a place, so the first match decides.
func Mark(results, base []Result) {
	for i, result := range results {
		match := slices.IndexFunc(base, func(old Result) bool {
			return old.Rule == result.Rule && slices.Equal(old.About, result.About)
		})
		switch {
		case match < 0:
			results[i].Change = ChangeNew
		case result.Value > base[match].Value:
			results[i].Change = ChangeWorse
		default:
			results[i].Change = ChangeUnchanged
		}
	}
}
