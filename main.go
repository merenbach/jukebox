package main

import (
	"encoding/json"
	"errors"
	"fmt"
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
func ServePlaylist(rr []string) {
	// Sort resources alphabetically.
	sort.Slice(rr, func(i, j int) bool {
		return rr[i] < rr[j]
	})

	library := make(map[string]string)
	for _, r := range rr {
		library[r] = "hello"
	}

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
			// TODO: all responses should be JSON
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
			// TODO: all responses should be JSON
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		resourceName := path.Base(r.URL.Path)
		t := Track{
			Timestamp: time.Now().Unix(),
			Resource:  resourceName,
		}

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
			// TODO: all responses should be JSON
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		bb, err := json.Marshal(playlist.Tracks())
		if err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(w, "%s", bb)
	})

	/*http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!DOCTYPE html>
		<html lang="en">
		<head>
		<meta charset="utf-8">
		<title>Gorse</title>
		</head>
		<body>
		<button type="button" id="mybutton">Push</button>
		<script type="text/javascript">
		(function() {
			"use strict";

			var button = document.getElementById("mybutton");
			window.setInterval(function() {
				var sec = Date.now() / 1000;
				console.log("sec = " + sec);
				fetch('/timestamp')
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
					var pushed = sec < data.timestamp + 2;
					button.disabled = pushed;
				});
			}, 100);

			button.onclick = function() {
				console.log("Pushing the button");
				// button.disabled = true;
				fetch("/timestamp", {
					method: "POST",
				});
			};
		})();
		</script>
		</body>
		</html>`)
	})*/

	// TODO: set timeouts, max header bytes!
	// s := &http.Server{
	// 	Addr:           ":8080",
	// 	Handler:        myHandler,
	// 	ReadTimeout:    10 * time.Second,
	// 	WriteTimeout:   10 * time.Second,
	// 	MaxHeaderBytes: 1 << 20,
	// }
	// log.Fatal(s.ListenAndServe())

	log.Fatal(http.ListenAndServe(":8080", nil))
}

// time.Now().Unix()
func main() {
	validResources := []string{
		"56k",
		"deeper",
	}
	ServePlaylist(validResources)
}
