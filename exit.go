package main

import (
	"html/template"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/browser"
)

// Exit dumps the last 100 lines of output to a crash file in /tmp (or equivalent), and generates a prettier HTML file containing it that is opened in the browser if possible.
func Exit(err interface{}) {
	tmpl, err2 := template.ParseFS(localFS, "html/crash.html", "html/header.html")
	if err2 != nil {
		log.Fatalf("Failed to load template: %v", err)
	}
	logCache := lineCache.String()
	sanitized := sanitizeLog(logCache)
	data := map[string]interface{}{
		"Log":          logCache,
		"SanitizedLog": sanitized,
	}
	if err != nil {
		data["Err"] = err
	}
	fpath := filepath.Join(temp, "jfa-go-crash-"+time.Now().Local().Format("2006-01-02T15:04:05"))
	err2 = os.WriteFile(fpath+".txt", []byte(logCache), 0666)
	if err2 != nil {
		log.Fatalf("Failed to write crash dump file: %v", err2)
	}
	log.Printf("\n------\nA crash report has been saved to \"%s\".\n------", fpath+".txt")
	f, err2 := os.OpenFile(fpath+".html", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0666)
	if err2 != nil {
		log.Fatalf("Failed to open crash dump file: %v", err2)
	}
	defer f.Close()
	err2 = tmpl.Execute(f, data)
	if err2 != nil {
		log.Fatalf("Failed to execute template: %v", err2)
	}
	browser.OpenFile(fpath + ".html")
}
