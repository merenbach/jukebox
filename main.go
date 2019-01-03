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

// A StringQueue is a thread-safe queue of strings.
type StringQueue struct {
	elements     []string
	elementsLock sync.RWMutex
}

// NewStringQueue returns a new StringQueue struct pointer.
func NewStringQueue() *StringQueue {
	return &StringQueue{
		elements: []string{},
	}
}

// Elements in the queue.
func (sq *StringQueue) Elements() []string {
	sq.elementsLock.RLock()
	defer sq.elementsLock.RUnlock()

	return sq.elements
}

// Count the number of elements in the queue.
func (sq *StringQueue) Count() int {
	sq.elementsLock.RLock()
	defer sq.elementsLock.RUnlock()

	return len(sq.elements)
}

// Push an element on to the end of the queue and return it.
func (sq *StringQueue) Push(s string) string {
	sq.elementsLock.Lock()
	defer sq.elementsLock.Unlock()

	sq.elements = append(sq.elements, s)
	return s
}

// ShiftMany pops the given number of elements from the front of the queue and returns them.
func (sq *StringQueue) ShiftMany(i int) []string {
	sq.elementsLock.Lock()
	defer sq.elementsLock.Unlock()

	popped, remaining := sq.elements[:i], sq.elements[i:]
	sq.elements = remaining
	return popped
}

// A BatchStringQueue is a thread-safe queue of strings that
type BatchStringQueue struct {
	*StringQueue
	lastBatchSize      int
	lastBatchTimestamp int64
}

// ShiftBatch shifts the last batch count. Repeating the operation twice in a row will empty the queue.
func (sq *BatchStringQueue) ShiftBatch() []string {
	els := sq.ShiftMany(sq.lastBatchSize)
	sq.lastBatchSize = sq.Count()
	sq.lastBatchTimestamp = time.Now().Unix()
	return els
}

// NewBatchStringQueue returns a new StringQueue struct pointer.
func NewBatchStringQueue() *BatchStringQueue {
	return &BatchStringQueue{
		StringQueue: NewStringQueue(),
	}
}

// A Playlist contains an ordered list of tracks to play.
type Playlist struct {
	TrackLibrary map[string]string `json:"library"`
	selections   *BatchStringQueue
}

// NewPlaylist creates a new Playlist with the given timeout.
func NewPlaylist(library map[string]string, s int64) *Playlist {
	return &Playlist{
		selections:   NewBatchStringQueue(),
		TrackLibrary: library,
	}
}

// Prune old items from the event queue.
func (p *Playlist) Prune() {
	log.Println("Pruning...")
	pruned := p.selections.ShiftBatch()
	log.Println("Pruned the following elements:", pruned)
}

// Append a new Track to the end of a Playlist.
func (p *Playlist) Append(t string) string {
	if _, ok := p.TrackLibrary[t]; !ok {
		log.Print("invalid track selection:", t)
		return ""
	}

	e := p.selections.Push(t)
	log.Println("Pushed the following element:", e)

	return e
}

// Selections lists the current queue of track selections from the playlist.
func (p *Playlist) Selections() (int64, []string) {
	el := p.selections.Elements()
	return p.selections.lastBatchTimestamp, el
}

// Commands lists all available library entries.
func (p *Playlist) Commands() []string {
	ss := []string{}
	for k := range p.TrackLibrary {
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
		ticker := time.NewTicker(time.Second * defaultExpireSeconds)
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

		bb, err := json.Marshal(playlist.TrackLibrary)
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
		log.Println("Requested to play track:", resourceName)

		t := playlist.Append(resourceName)
		if t == "" {
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

		ts, sels := playlist.Selections()
		playlistData := struct {
			Timestamp  int64    `json:"timestamp"`
			Selections []string `json:"selections"`
		}{
			Timestamp:  ts,
			Selections: sels,
		}
		bb, err := json.Marshal(playlistData)
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
					//console.log("Running over everything...")
					var ts = data.timestamp;
					var sels = data.selections;
					console.log("ts = " + ts + " and sels = " + sels);
					for (var val of sels) {
						console.log("Evaluating whether to play " + JSON.stringify(val));
						var audio = document.getElementById('audio_' + val);
						if (ts > Number(audio.dataset.timestamp)) {
							audio.dataset.timestamp = ts;
							/*var audio = sounds.querySelector('[data-sound="' + rsrc + '"]');
							if (audio == null) {
								audio = new Audio('/sounds/' + rsrc);
								sounds.appendChild(audio);
								audio.dataset.sound = rsrc;
							}*/
							console.log('Playing ' + val);
							audio.play();
						} else {
							console.log("Already played selection " + val);
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

func main() {
	bb, err := ioutil.ReadFile("sounds.json")
	if err != nil {
		log.Fatal(err)
	}

	var resources map[string]string
	json.Unmarshal(bb, &resources)
	log.Println("Initializing with the following resources:", resources)

	/*snddir := "sounds"
	files, err := ioutil.ReadDir(snddir)
	if err != nil {
		log.Fatal(err)
	}

	library := []string{}
	for _, file := range files {
		library = append(library, file.Name())
	}*/

	ServePlaylist(resources)
}
