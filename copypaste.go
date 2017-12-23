package main

import (
	"encoding/json"
	"go/format"
	"log"
	"net/http"
	"strings"
	"text/template"

	"golang.org/x/tools/godoc"
	"golang.org/x/tools/godoc/redirect"
	"golang.org/x/tools/godoc/vfs"
)

// golang.org/x/tools/cmd/godoc/handlers.go

var (
	pres *godoc.Presentation
	fs   = vfs.NameSpace{}
)

func registerHandlers(pres *godoc.Presentation) *http.ServeMux {
	if pres == nil {
		panic("nil Presentation")
	}
	mux := http.NewServeMux()
	//mux.HandleFunc("/doc/codewalk/", codewalk)
	mux.Handle("/doc/play/", pres.FileServer())
	mux.Handle("/robots.txt", pres.FileServer())
	mux.Handle("/", pres)
	mux.Handle("/pkg/C/", redirect.Handler("/cmd/cgo/"))
	mux.HandleFunc("/fmt", fmtHandler)
	redirect.Register(mux)

	//http.Handle("/", hostEnforcerHandler{mux})

	return mux
}

func readTemplate(name string) *template.Template {
	if pres == nil {
		panic("no global Presentation set yet")
	}
	path := "lib/godoc/" + name

	// use underlying file system fs to read the template file
	// (cannot use template ParseFile functions directly)
	data, err := vfs.ReadFile(fs, path)
	if err != nil {
		log.Fatal("readTemplate: ", err)
	}
	// be explicit with errors (for app engine use)
	t, err := template.New(name).Funcs(pres.FuncMap()).Parse(string(data))
	if err != nil {
		log.Fatal("readTemplate: ", err)
	}
	return t
}

func readTemplates(p *godoc.Presentation, html bool) {
	p.PackageText = readTemplate("package.txt")
	p.SearchText = readTemplate("search.txt")

	if html || p.HTMLMode {
		//codewalkHTML = readTemplate("codewalk.html")
		//codewalkdirHTML = readTemplate("codewalkdir.html")
		p.CallGraphHTML = readTemplate("callgraph.html")
		p.DirlistHTML = readTemplate("dirlist.html")
		p.ErrorHTML = readTemplate("error.html")
		p.ExampleHTML = readTemplate("example.html")
		p.GodocHTML = readTemplate("godoc.html")
		p.ImplementsHTML = readTemplate("implements.html")
		p.MethodSetHTML = readTemplate("methodset.html")
		p.PackageHTML = readTemplate("package.html")
		p.SearchHTML = readTemplate("search.html")
		p.SearchDocHTML = readTemplate("searchdoc.html")
		p.SearchCodeHTML = readTemplate("searchcode.html")
		p.SearchTxtHTML = readTemplate("searchtxt.html")
		p.SearchDescXML = readTemplate("opensearch.xml")
	}
}

type fmtResponse struct {
	Body  string
	Error string
}

// fmtHandler takes a Go program in its "body" form value, formats it with
// standard gofmt formatting, and writes a fmtResponse as a JSON object.
func fmtHandler(w http.ResponseWriter, r *http.Request) {
	resp := new(fmtResponse)
	body, err := format.Source([]byte(r.FormValue("body")))
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Body = string(body)
	}
	w.Header().Set("Content-type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(resp)
}

// golang.org/x/tools/cmd/godoc/index.go
func indexDirectoryDefault(dir string) bool {
	return dir != "/pkg" && !strings.HasPrefix(dir, "/pkg/")
}