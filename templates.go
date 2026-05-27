package main

import (
	"html/template"
)

type breadcrumb struct {
	Name string
	Path string
}

type pageData struct {
	Title       string
	CSS         template.CSS
	Body        template.HTML
	Breadcrumbs []breadcrumb
	HasMermaid  bool
}

type entryInfo struct {
	Name    string
	URL     string
	IsDir   bool
	Size    string
	ModTime string
}

type dirData struct {
	Path       string
	CSS        template.CSS
	HasParent  bool
	ParentPath string
	Entries    []entryInfo
}

var mdTmpl = template.Must(template.New("markdown").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>{{.CSS}}</style>
</head>
<body>
<div class="mdview-container">
<nav class="mdview-breadcrumb">
<a href="/">root</a>
{{range .Breadcrumbs}} / <a href="{{.Path}}">{{.Name}}</a>{{end}}
</nav>
<article class="markdown-body">
{{.Body}}
</article>
{{if .HasMermaid}}<script src="/__mdview/mermaid.js"></script>
<script>mermaid.initialize({startOnLoad:false,theme:"default"});mermaid.run();</script>{{end}}
</div>
</body>
</html>`))

var dirTmpl = template.Must(template.New("dirlist").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Index of {{.Path}}</title>
<style>{{.CSS}}</style>
</head>
<body>
<div class="mdview-container">
<h1 style="margin-top:0">Index of {{.Path}}</h1>
<table class="dir-list">
<thead><tr><th>Name</th><th class="size">Size</th><th class="modified">Modified</th></tr></thead>
<tbody>
{{if .HasParent}}<tr><td><a href="{{.ParentPath}}">../</a></td><td class="size">&mdash;</td><td class="modified">&mdash;</td></tr>{{end}}
{{range .Entries}}
<tr>
<td><a href="{{.URL}}">{{if .IsDir}}&#128193; {{end}}{{.Name}}{{if .IsDir}}/{{end}}</a></td>
<td class="size">{{if .IsDir}}&mdash;{{else}}{{.Size}}{{end}}</td>
<td class="modified">{{.ModTime}}</td>
</tr>
{{end}}
</tbody>
</table>
</div>
</body>
</html>`))
