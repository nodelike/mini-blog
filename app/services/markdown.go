package services

import (
	"html/template"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

func MarkdownToHTML(markdownText string) template.HTML {
	if markdownText == "" {
		return template.HTML("")
	}

	extensions := parser.CommonExtensions | parser.AutoHeadingIDs
	p := parser.NewWithExtensions(extensions)

	opts := html.RendererOptions{
		Flags: html.CommonFlags | html.HrefTargetBlank,
	}
	renderer := html.NewRenderer(opts)

	htmlBytes := markdown.ToHTML([]byte(markdownText), p, renderer)
	return template.HTML(htmlBytes)
}
