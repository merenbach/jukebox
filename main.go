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
// TODO: unique code id for sounds so we can put into "id" attribute instead of data attribute? or use for data anyway?
// TOOD: how are we cleaning up sound names? can we read ID3 tags?? but what of wavs? trim extension?

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

// A Track represents a timestamped resource invocation in a playlist.
type Track struct {
	Resource  string `json:"resource"`
	Timestamp int64  `json:"timestamp"`
}

// NewerThan determines if this Track is newer than a given number of seconds.
func (e Track) newerThan(s int64) bool {
	t := time.Now().Unix()
	return t-e.Timestamp < s
}

// A Playlist contains an ordered list of tracks to play.
type Playlist struct {
	Timeout    time.Duration     `json:"timeout"`
	Library    map[string]string `json:"library"`
	tracks     []Track
	tracksLock sync.RWMutex
}

// NewPlaylist creates a new Playlist with the given timeout.
func NewPlaylist(library map[string]string, s int64) *Playlist {
	return &Playlist{
		tracks:  []Track{},
		Timeout: time.Duration(s),
		Library: library,
	}
}

// Prune old items from the event queue.
func (p *Playlist) Prune() {
	fmt.Println("Pruning...")

	p.tracksLock.Lock()
	tt := p.tracks[:0]
	for i, e := range p.tracks {
		if e.newerThan(int64(p.Timeout)) {
			tt = p.tracks[i:]
			break
		}
	}
	fmt.Println("Pruned to:", tt)
	p.tracks = tt

	p.tracksLock.Unlock()
}

// Append a new Track to the end of a Playlist.
func (p *Playlist) Append(t Track) error {
	if _, ok := p.Library[t.Resource]; !ok {
		return errors.New("invalid track")
	}

	p.tracksLock.Lock()
	p.tracks = append(p.tracks, t)
	p.tracksLock.Unlock()

	return nil
}

// Tracks lists tracks in the playlist.
func (p *Playlist) Tracks() []Track {
	p.tracksLock.RLock()
	tt := make([]Track, len(p.tracks))
	copy(tt, p.tracks)
	p.tracksLock.RUnlock()

	return tt
}

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
	eq.tracksLock.RLock()
	for i, e := range eq.Events {
		if e.newerThan(int64(eq.Timeout)) {
			out = eq.Events[i:]
			break
		}
	}
	eq.tracksLock.RUnlock()
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
		t := Track{
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
		fmt.Fprint(w, `<!DOCTYPE html>
		<html lang="en">
		<head>
		<meta charset="utf-8">
		<title>Jukebox</title>
		</head>
		<body>`)

		//fmt.Fprint(w, "<div id=\"sounds\"></div>")
		fmt.Fprint(w, "<ul>")
		for _, s := range playlist.Commands() {
			// TODO: support multiple formats for same name??
			fmt.Fprint(w, "<li>")
			//audioPath := path.Join("/sounds/", s)
			fmt.Fprintf(w, "<audio preload=\"auto\" src=\"%s\" id=\"audio_%s\"></audio>", library[s], s)
			fmt.Fprintf(w, "<button class=\"play-button\" data-sound=\"%s\">%s</button>", s, s)
			fmt.Fprint(w, "</li>")
		}
		fmt.Fprint(w, "</ul>")
		fmt.Fprint(w, `<script type="text/javascript">
		(function() {
			"use strict";

			function setupLibrary() {
				var els = document.getElementsByClassName("play-button");
				for (var i = 0; i < els.length; i++) {
					const element = els[i];
					element.onclick = function() {
						const snd = element.dataset.sound;
						fetch('/play/' + snd, {
							method: "POST",
						});
						//document.getElementById('audio_' + snd).play();
					};
				}
			}

			setupLibrary();
			
			//var button = document.getElementById("mybutton");
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
					//const sounds = document.getElementById("sounds");
					for (var val of data) {
						var rsrc = val.resource;
						var audio = document.getElementById('audio_' + rsrc);
						/*var audio = sounds.querySelector('[data-sound="' + rsrc + '"]');
						if (audio == null) {
							audio = new Audio('/sounds/' + rsrc);
							sounds.appendChild(audio);
							audio.dataset.sound = rsrc;
						}*/
						console.log('Playing ' + rsrc);
						audio.play();
						
						//document.getElementById('audio_' + val.resource).play();
					}
				});
			}, 100);

			/*button.onclick = function() {
				console.log("Pushing the button");
				document.getElementById('hello').play();
				// button.disabled = true;
				fetch("/play/56k", {
					method: "POST",
				});
			};*/
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
