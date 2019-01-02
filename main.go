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

// TODO: support multiple audio formats and don't tightly couple to MP3
// TODO: emoji responses? handles for participants?
// TODO: rename Playlist => Library?
// TODO: replace Track nomenclature

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"sort"
	"sync"
	"time"
)

// Remove from the queue any events older than this.
// This mitigates potential for endless queue growth.
const defaultExpireSeconds = 5

// // A Resource represents the details of a resource in the queue.
// type Resource struct {
// 	Name string `json:"name"`
// 	Path string `json:"path"`
// }

// A Selection is a request to play something in the library.
type Selection struct {
	Resource  string `json:"resource"`
	Timestamp int64  `json:"timestamp"`
}

// NewerThan determines if this Track is newer than a given number of seconds.
func (s Selection) newerThan(ts int64) bool {
	now := time.Now().Unix()
	return now-s.Timestamp < ts
}

// A Playlist contains an ordered list of tracks to play.
type Playlist struct {
	Timeout        time.Duration     `json:"timeout"`
	Library        map[string]string `json:"library"`
	selections     []Selection
	selectionsLock sync.RWMutex
}

// NewPlaylist creates a new Playlist with the given timeout.
func NewPlaylist(library map[string]string, s int64) *Playlist {
	return &Playlist{
		selections: []Selection{},
		Timeout:    time.Duration(s),
		Library:    library,
	}
}

// Prune old items from the event queue.
func (p *Playlist) Prune() {
	fmt.Println("Pruning...")

	p.selectionsLock.Lock()
	tt := p.selections[:0]
	for i, e := range p.selections {
		if e.newerThan(int64(p.Timeout)) {
			tt = p.selections[i:]
			break
		}
	}
	fmt.Println("Pruned to:", tt)
	p.selections = tt

	p.selectionsLock.Unlock()
}

// Append a new Track to the end of a Playlist.
func (p *Playlist) Append(t Selection) error {
	if _, ok := p.Library[t.Resource]; !ok {
		return errors.New("invalid track")
	}

	p.selectionsLock.Lock()
	p.selections = append(p.selections, t)
	p.selectionsLock.Unlock()

	return nil
}

// Tracks lists tracks in the playlist.
func (p *Playlist) Tracks() []Selection {
	p.selectionsLock.RLock()
	tt := make([]Selection, len(p.selections))
	copy(tt, p.selections)
	p.selectionsLock.RUnlock()

	return tt
}

// Commands lists all available library entries.
func (p *Playlist) Commands() []string {
	ss := []string{}
	for k := range p.Library {
		ss = append(ss, k)
	}

	// Sort resources alphabetically.
	sort.Slice(ss, func(i, j int) bool {
		return ss[i] < ss[j]
	})

	return ss
}

/*
// Filtered events from the queue.
func (eq *EventQueue) Filtered() []Event {
	var out []Event
	eq.selectionsLock.RLock()
	for i, e := range eq.Events {
		if e.newerThan(int64(eq.Timeout)) {
			out = eq.Events[i:]
			break
		}
	}
	eq.selectionsLock.RUnlock()
	if out == nil {
		out = []Event{}
	}
	return out
}*/

// ServePlaylist runs an HTTP server with a playlist queue.
func ServePlaylist(library map[string]string) {

	playlist := NewPlaylist(library, defaultExpireSeconds)

	// [TODO]: Replace with a minute instead?
	go func(p *Playlist) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for range ticker.C {
			p.Prune()
		}
	}(playlist)

	http.HandleFunc("/library/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "{}", http.StatusMethodNotAllowed)
			return
		}

		bb, err := json.Marshal(playlist.Library)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(w, "%s", bb)
		return
	})

	http.HandleFunc("/play/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "{}", http.StatusMethodNotAllowed)
			return
		}

		resourceName := path.Base(r.URL.Path)
		t := Selection{
			Timestamp: time.Now().Unix(),
			Resource:  resourceName,
		}

		fmt.Println("Requested to play track:", t)

		err := playlist.Append(t)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		bb, err := json.Marshal(t)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(w, "%s", bb)
	})

	http.HandleFunc("/playlist/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "{}", http.StatusMethodNotAllowed)
			return
		}

		bb, err := json.Marshal(playlist.Tracks())
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(w, "%s", bb)
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		//<link href="https://necolas.github.io/normalize.css/8.0.1/normalize.css" rel="stylesheet">
		fmt.Fprint(w, `<!DOCTYPE html>
		<html lang="en">
		<head>
		<meta charset="utf-8">
		<title>Jukebox</title>
		<style>
			body {
				background-color: #333;
				color: #ccc;
			}

			#selections {
				padding-left: 0;
			}

			#selections .selection {
				display: inline-block;
				padding: .25em .5em;
			}

			#selections .play {
				color: #8f8;
			}
		</style>
		</head>
		<body>
			<h1>Jukebox</h1>
			<p>Play sounds for everyone who has this page loaded!</p>
		`)

		//fmt.Fprint(w, "<div id=\"sounds\"></div>")
		fmt.Fprint(w, "<ul id=\"selections\">")
		for _, s := range playlist.Commands() {
			fmt.Fprint(w, "<li class=\"selection\">")
			fmt.Fprintf(w, "<a class=\"play\" data-sound=\"%s\" href=\"#\">%s</a>", s, s)
			fmt.Fprintf(w, "<audio preload=\"auto\" src=\"%s\" id=\"audio_%s\" data-timestamp=\"\">Your browser does not support the <code>audio</code> element.</audio>", library[s], s)
			fmt.Fprint(w, "</li>")
		}
		fmt.Fprint(w, "</ul>")
		fmt.Fprint(w, `<script>
		(function() {
			"use strict";

			Array.from(document.getElementsByClassName("play")).forEach( (e) => e.addEventListener("click", function(event) {
				event.preventDefault();
				const snd = e.dataset.sound;
				fetch('/play/' + snd, {
					method: "POST",
				});
			}, false));
			
			window.setInterval(function() {
				fetch('/playlist/')
				.catch(function(e) {
					console.log(e);
				})
				.then(function(response) {
					if (response.ok) {
						return response.json();
					}
					throw new Error(response.statusText);
				})
				.catch(function(e) {
					console.log(e);
				})
				.then(function(data) {
					for (var val of data) {
						console.log("Evaluating whether to play " + JSON.stringify(val));
						var audio = document.getElementById('audio_' + val.resource);
						if (val.timestamp > Number(audio.dataset.timestamp)) {
							audio.dataset.timestamp = val.timestamp;
							/*var audio = sounds.querySelector('[data-sound="' + rsrc + '"]');
							if (audio == null) {
								audio = new Audio('/sounds/' + rsrc);
								sounds.appendChild(audio);
								audio.dataset.sound = rsrc;
							}*/
							console.log('Playing ' + val.resource);
							audio.play();
						} else {
							console.log("Already played selection " + val.resource);
						}
					}
				});
			}, 100);
		})();
		</script>
		</body>
		</html>`)
	})

	// TODO: set timeouts, max header bytes!
	// s := &http.Server{
	// 	Addr:           ":8080",
	// 	Handler:        myHandler,
	// 	ReadTimeout:    10 * time.Second,
	// 	WriteTimeout:   10 * time.Second,
	// 	MaxHeaderBytes: 1 << 20,
	// }
	// log.Fatal(s.ListenAndServe())

	fs := http.FileServer(http.Dir("sounds"))
	http.Handle("/sounds/", http.StripPrefix("/sounds/", fs))

	log.Fatal(http.ListenAndServe(":8080", nil))
}

// time.Now().Unix()
func main() {
	bb, err := ioutil.ReadFile("sounds.json")
	if err != nil {
		log.Fatal(err)
	}

	var library map[string]string
	json.Unmarshal(bb, &library)
	fmt.Println(library)

	/*snddir := "sounds"
	files, err := ioutil.ReadDir(snddir)
	if err != nil {
		log.Fatal(err)
	}

	library := []string{}
	for _, file := range files {
		library = append(library, file.Name())
	}*/

	ServePlaylist(library)
}
