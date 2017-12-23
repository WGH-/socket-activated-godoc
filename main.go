package main

import (
	"archive/zip"
	"context"
	_ "expvar" // to serve /debug/vars
	"flag"
	"go/build"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof" // to serve /debug/pprof/*
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/tools/godoc"
	"golang.org/x/tools/godoc/analysis"
	"golang.org/x/tools/godoc/static"
	"golang.org/x/tools/godoc/vfs"
	"golang.org/x/tools/godoc/vfs/gatefs"
	"golang.org/x/tools/godoc/vfs/mapfs"
	"golang.org/x/tools/godoc/vfs/zipfs"

	"github.com/coreos/go-systemd/activation"
)

const defaultAddr = ":6060" // default webserver address

var (
	zipfile = flag.String("zip", "", "zip file providing the file system to serve; disabled if empty")

	analysisFlag = flag.String("analysis", "", `comma-separated list of analyses to perform (supported: type, pointer). See http://golang.org/lib/godoc/analysis/help.html`)

	httpAddr = flag.String("http", defaultAddr, "HTTP service address (e.g., '"+defaultAddr+"')")

	inactivityTimeout = flag.Duration("inactivity_timeout", 5*time.Minute, "Inactivity timeout for socket activation")

	verbose = flag.Bool("v", false, "verbose mode")

	goroot = flag.String("goroot", runtime.GOROOT(), "Go root directory")

	// layout control
	tabWidth       = flag.Int("tabwidth", 4, "tab width")
	showTimestamps = flag.Bool("timestamps", false, "show timestamps with directory listings")
	declLinks      = flag.Bool("links", true, "link identifiers to their declarations")

	templateDir = flag.String("templates", "", "load templates/JS/CSS from disk in this directory")

	// search index
	indexEnabled  = flag.Bool("index", false, "enable search index")
	indexFiles    = flag.String("index_files", "", "glob pattern specifying index files; if not empty, the index is read from these files in sorted order")
	indexInterval = flag.Duration("index_interval", 0, "interval of indexing; 0 for default (5m), negative to only index once at startup")
	maxResults    = flag.Int("maxresults", 10000, "maximum number of full text search results shown")
	indexThrottle = flag.Float64("index_throttle", 0.75, "index throttle value; 0.0 = no time allocated, 1.0 = full throttle")

	// source code notes
	notesRx = flag.String("notes", "BUG", "regular expression matching note markers to show")
)

func main() {
	flag.Parse()

	// most of the code is simply copy-pasted from golang.org/x/tools/cmd/godoc/main.go

	var fsGate chan bool
	fsGate = make(chan bool, 20)

	// Determine file system to use.
	if *zipfile == "" {
		// use file system of underlying OS
		rootfs := gatefs.New(vfs.OS(*goroot), fsGate)
		fs.Bind("/", rootfs, "/", vfs.BindReplace)
	} else {
		// use file system specified via .zip file (path separator must be '/')
		rc, err := zip.OpenReader(*zipfile)
		if err != nil {
			log.Fatalf("%s: %s\n", *zipfile, err)
		}
		defer rc.Close() // be nice (e.g., -writeIndex mode)
		fs.Bind("/", zipfs.New(rc, *zipfile), *goroot, vfs.BindReplace)
	}
	if *templateDir != "" {
		fs.Bind("/lib/godoc", vfs.OS(*templateDir), "/", vfs.BindBefore)
	} else {
		fs.Bind("/lib/godoc", mapfs.New(static.Files), "/", vfs.BindReplace)
	}

	// Bind $GOPATH trees into Go root.
	for _, p := range filepath.SplitList(build.Default.GOPATH) {
		fs.Bind("/src", gatefs.New(vfs.OS(p), fsGate), "/src", vfs.BindAfter)
	}

	var typeAnalysis, pointerAnalysis bool
	if *analysisFlag != "" {
		for _, a := range strings.Split(*analysisFlag, ",") {
			switch a {
			case "type":
				typeAnalysis = true
			case "pointer":
				pointerAnalysis = true
			default:
				log.Fatalf("unknown analysis: %s", a)
			}
		}
	}

	corpus := godoc.NewCorpus(fs)
	corpus.Verbose = *verbose
	corpus.MaxResults = *maxResults
	corpus.IndexEnabled = *indexEnabled
	if *maxResults == 0 {
		corpus.IndexFullText = false
	}
	corpus.IndexFiles = *indexFiles
	corpus.IndexDirectory = indexDirectoryDefault
	corpus.IndexThrottle = *indexThrottle
	corpus.IndexInterval = *indexInterval

	if err := corpus.Init(); err != nil {
		log.Fatal(err)
	}

	// N.B. global variable defined in adjacent file
	pres = godoc.NewPresentation(corpus)
	pres.TabWidth = *tabWidth
	pres.ShowTimestamps = *showTimestamps
	pres.ShowPlayground = false
	pres.DeclLinks = *declLinks
	if *notesRx != "" {
		pres.NotesRx = regexp.MustCompile(*notesRx)
	}

	readTemplates(pres, true)
	handler := registerHandlers(pres)

	http.Handle("/", handler)

	// Initialize search index.
	if *indexEnabled {
		go corpus.RunIndexer()
	}

	// Start type/pointer analysis.
	if typeAnalysis || pointerAnalysis {
		go analysis.Run(pointerAnalysis, &corpus.Analysis)
	}

	listeners, err := activation.Listeners(true)
	if err != nil {
		log.Fatal(err)
	}

	if *verbose {
		log.Printf("Go Documentation Server")
		log.Printf("version = %s", runtime.Version())
		log.Printf("goroot = %s", *goroot)
		log.Printf("tabwidth = %d", *tabWidth)
	}

	var ln net.Listener

	server := &http.Server{}

	switch len(listeners) {
	case 0:
		var err error
		ln, err = net.Listen("tcp", *httpAddr)
		if err != nil {
			log.Fatal("Failed to listen ", err)
		}
		if *verbose {
			log.Printf("address = %s", *httpAddr)
		}
	case 1:
		ln = listeners[0]
		if *verbose {
			log.Printf("address (socket-activated) = %s", ln.Addr())
		}

		h := newLastActivityHTTPHandler(server.Handler, *inactivityTimeout)
		server.Handler = h
		go func() {
			var err error
			<-h.timer.C
			log.Print("HTTP inactivity timeout, shutting down")

			ctx, _ := context.WithTimeout(context.TODO(), time.Second*30)
			err = server.Shutdown(ctx)
			if err != nil {
				log.Print("Error during server shutdown: ", err)
			}
			err = server.Close()
			if err != nil {
				log.Print("Error during server close: ", err)
			}
		}()
	default:
		log.Fatal("Unexpected number of sockets passed from systemd: ", len(listeners))
	}

	err = server.Serve(ln)
	if err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
