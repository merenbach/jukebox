// Most of the code in this file...
// Copyright 2018 Andrew Merenbach
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Other portions...
// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// TODO: emoji responses? handles for participants?
// TODO: revamp fault tolerance (invalid sound, etc.)
// TODO: better log/history display in browser, plus status messages about joins/leaves--and don't try to play those...
// TODO: Lambda to run? Accept URI for sound library...
// TODO: different rooms, namespaced to allow multiple "conversations"
// TODO: Slack integration
// TODO: dedicated client app to submit?
// NOTE: portions based heavily on https://github.com/gorilla/websocket/tree/master/examples/chat
// TODO: allow refreshing of list if remote manifest updated??
// TODO: remove trailing /ws if we can switch to Heroku for POC

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"path/filepath"
)

var addr = flag.String("addr", "localhost:8080", "http service address")
var manifest = flag.String("manifest", "", "URL of sound library JSON")

// GetRemoteFile reads the contents of a file from a remote URL.
func getRemoteFile(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()
	return ioutil.ReadAll(resp.Body)
}

func main() {
	flag.Parse()
	hub := newHub()
	go hub.run()

	log.Println("Initializing with address: ", *addr)
	log.Println("Initializing with manifest: ", *manifest)
	var library map[string]string

	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})
	// TODO: improve this....
	http.HandleFunc("/play/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if library == nil {
				bb, err := getRemoteFile(*manifest)
				if err != nil {
					log.Fatal(err)
				}
				if err = json.Unmarshal(bb, &library); err != nil {
					log.Fatal(err)
				}
			}
			bb, err := json.Marshal(library)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Fprintf(w, "%s", bb)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		resourceName := path.Base(r.URL.Path)
		log.Println("Requested to play sound:", resourceName)
		hub.broadcast <- []byte(resourceName)
	})
	// <<----
	// TODO: remove from final product--->
	fs := http.FileServer(http.Dir("static"))
	http.Handle("/static/", http.StripPrefix("/static/", fs))
	// <<<<<----
	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}

	//log.Fatal(http.ListenAndServe(*addr, nil))
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	log.Println(r.URL)
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	/*p := &Page{
		Title: "You are invited",
		Event: myEvent,
		Group: *myEvent.groupForToken(token),
	}*/
	//fmt.Fprintf(w, "<h1>%s</h1><div>%s</div>", p.Title, p.Body)
	renderHTMLTemplate(w, "main")

	//homeTemplate.Execute(w, "ws://"+r.Host+"/ws/")
}

func renderHTMLTemplate(w http.ResponseWriter, tmpl string) {
	layoutPath := filepath.Join("templates", "layout.html")
	bodyPath := filepath.Join("templates", tmpl+".html")
	t := template.Must(template.ParseFiles(layoutPath, bodyPath))
	if err := t.Execute(w, ""); err != nil {
		log.Fatal("An error occurred: ", err)
	}
}

//var homeTemplate = template.Must(template.New("").Parse(``))
