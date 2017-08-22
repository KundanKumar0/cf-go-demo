// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/cloudfoundry-community/go-cfenv"
	"gopkg.in/redis.v4"
	"html/template"
	"log"
	"net/http"
	"os"
	"regexp"
)

type Page struct {
	Title    string
	Instance string
	Body     []byte
}

func loadPage(title string) (*Page, error) {
	p := Page{Title: title}
	c := redis.NewClient(redisOptions())
	defer c.Close()
	s, err := c.Get(title).Result()
	if err != nil {
		fmt.Println("Error loading file: ", title)
		return &p, err
	} else {
		p = FromGOB([]byte(s))
	}
	return &p, nil
}

func saveHandler(w http.ResponseWriter, r *http.Request, title string) {
	body := r.FormValue("body")
	title = title
	p := Page{Title: title, Body: []byte(body)}

	//save to Redis
	c := redis.NewClient(redisOptions())
	defer c.Close()
	err := c.Set(title, ToGOB(p), 0).Err()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/view/"+title, http.StatusFound)
}

var templates = template.Must(template.ParseFiles("edit.html", "view.html"))

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
	p.Instance = os.Getenv("CF_INSTANCE_INDEX")
	err := templates.ExecuteTemplate(w, tmpl+".html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var validPath = regexp.MustCompile("^/(edit|save|view)/([a-zA-Z0-9]+)$")

func makeHandler(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		fn(w, r, m[2])
	}
}

func redisOptions() *redis.Options {
	appEnv, err := cfenv.Current()
	if err != nil {
		log.Fatal("VCAP_SERVICES and VCAP_APPLICATION must be set!")
	}
	r, err := appEnv.Services.WithName("my-redis")
	if err != nil {
		log.Fatal(err)
	}
	hostname, _ := r.CredentialString("hostname")
	port, _ := r.CredentialString("port")
	password, _ := r.CredentialString("password")
	return &redis.Options{
		Addr:     hostname + ":" + port,
		Password: password,
		DB:       0, // use default DB
	}
}

func ToGOB(p Page) []byte {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(p)
	if err != nil {
		fmt.Println("failed gob Encode", err)
	}
	return b.Bytes()
}

func FromGOB(by []byte) Page {
	p := Page{}
	b := bytes.Buffer{}
	b.Write(by)
	d := gob.NewDecoder(&b)
	err := d.Decode(&p)
	if err != nil {
		fmt.Println("failed gob Decode", err)
	}
	return p
}
func main() {
	http.HandleFunc("/view/", makeHandler(viewHandler))
	http.HandleFunc("/edit/", makeHandler(editHandler))
	http.HandleFunc("/save/", makeHandler(saveHandler))

	http.ListenAndServe(":8080", nil)
}

func viewHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(title)

	if err != nil {
		http.Redirect(w, r, "/edit/"+title, http.StatusFound)
		return
	}
	renderTemplate(w, "view", p)
}

func editHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, _ := loadPage(title)
	renderTemplate(w, "edit", p)
}
