package model

import (
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// URL returns the editor link for a line of a file, with file relative to Dir as in the model.
func (s Source) URL(file string, line int) string {
	absolute := filepath.ToSlash(filepath.Join(s.Dir, filepath.FromSlash(file)))
	return strings.NewReplacer("{file}", encodePath(absolute), "{line}", strconv.Itoa(line)).Replace(s.EditorURL)
}

// encodePath must fit both a URL path and a query value, because the template decides where {file}
// goes. url.PathEscape leaves & and + bare, which breaks a query. QueryEscape's + for a space is
// wrong in a path, so it becomes %20; a real + is already %2B by then.
func encodePath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = strings.ReplaceAll(url.QueryEscape(segment), "+", "%20")
	}
	return strings.Join(segments, "/")
}
