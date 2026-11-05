// Package page writes the godeps model as one HTML file that needs no network.
package page

import (
	_ "embed"
	"html/template"
	"io"

	"github.com/n-valchev/goalie/internal/model"
)

//go:embed page.html
var pageHTML string

//go:embed page.css
var pageCSS string

//go:embed page.js
var pageJS string

var pageTemplate = template.Must(template.New("page").Parse(pageHTML))

// Only our own embedded files take the trusted types. Model stays a plain value, because the escaper
// is what keeps its text inside the model element.
type content struct {
	Title  string
	Style  template.CSS
	Script template.JS
	Model  model.Model
}

func Write(w io.Writer, result model.Model) error {
	return pageTemplate.Execute(w, content{
		Title:  result.Title,
		Style:  template.CSS(pageCSS),
		Script: template.JS(pageJS),
		Model:  result,
	})
}
