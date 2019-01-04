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
// TODO: revamp fault tolerance (invalid sound, etc.)

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

	//<link href="https://necolas.github.io/normalize.css/8.0.1/normalize.css" rel="stylesheet">

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

	flag.Parse()
	log.SetFlags(0)
	// http.HandleFunc("/echo", echo)
	//http.HandleFunc("/", home)
	log.Fatal(http.ListenAndServe(*addr, nil))

	//log.Fatal(http.ListenAndServe(":8080", nil))
}

var addr = flag.String("addr", "localhost:8080", "http service address")

/*func echo(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()
	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("recv: %s", message)

		trackuri, ok := playlist.TrackLibrary[string(message)]
		if !ok {
			log.Print("invalid track selection:", message)
			continue
		}
		err = c.WriteMessage(mt, []byte(trackuri))
		if err != nil {
			log.Println("write:", err)
			break
		}
	}
}*/

var playlist *Playlist

func main() {
	bb, err := ioutil.ReadFile("sounds.json")
	if err != nil {
		log.Fatal(err)
	}

	var resources map[string]string
	json.Unmarshal(bb, &resources)
	log.Println("Initializing with the following resources:", resources)

	library := resources
	playlist = NewPlaylist(library, defaultExpireSeconds)

	flag.Parse()
	hub := newHub()
	go hub.run()
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})
	// TODO: remove from final product--->
	fs := http.FileServer(http.Dir("sounds"))
	http.Handle("/sounds/", http.StripPrefix("/sounds/", fs))
	// <<<<<----
	err = http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}

	//flag.Parse()
	//log.SetFlags(0)
	//http.HandleFunc("/echo", echo)
	//http.HandleFunc("/", home)
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
	home(w, r)
	//http.ServeFile(w, r, "home.html")
}

func home(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Url      string
		Commands map[string]string
	}{
		// TODO: allow connecting to arbitrary sound machines!
		//Url:      "ws://" + r.Host + "/ws",
		Commands: playlist.TrackLibrary,
	}
	homeTemplate.Execute(w, data)
}

func main2() {
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

var homeTemplate = template.Must(template.New("").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<title>Chat Example</title>
<script type="text/javascript">
window.onload = function () {
    var conn;
    var msg = document.getElementById("msg");
    var log = document.getElementById("log");

    function appendLog(item) {
        var doScroll = log.scrollTop > log.scrollHeight - log.clientHeight - 1;
        log.appendChild(item);
        if (doScroll) {
            log.scrollTop = log.scrollHeight - log.clientHeight;
        }
    }

    document.getElementById("form").onsubmit = function () {
        if (!conn) {
            return false;
        }
        if (!msg.value) {
            return false;
        }
        conn.send(msg.value);
        msg.value = "";
        return false;
    };

	
	////>>
	Array.from(document.getElementsByClassName("play")).forEach( (e) => e.addEventListener("click", function(event) {
		event.preventDefault();
		if (!conn) {
			return false;
		}
		console.log("SEND: " + e.dataset.sound);
		conn.send(e.dataset.sound);
		return false;
	}, false));
	////<<

    if (window["WebSocket"]) {
		conn = new WebSocket("ws://" + document.location.host + "/ws");

        conn.onclose = function (evt) {
            var item = document.createElement("div");
            item.innerHTML = "<b>Connection closed.</b>";
            //appendLog(item);
        };
        conn.onmessage = function (evt) {
			var messages = evt.data.split('\n');

            for (var i = 0; i < messages.length; i++) {

				var rsrc = messages[i];
				var selections = document.getElementById('selections');
			var audio = selections.querySelector('audio[data-sound="' + rsrc + '"]');
			/*if (audio == null) {
				audio = new Audio(rsrc);
				sounds.appendChild(audio);
			}*/
			audio.play();
			/*try {
			audio.play();
			} catch (err) {
				console.log("Could not play sound: " + err)
			}*/

               // var item = document.createElement("div");
                //item.innerText = messages[i];
                //appendLog(item);
            }
        };
    } else {
        var item = document.createElement("div");
        item.innerHTML = "<b>Your browser does not support WebSockets.</b>";
        //appendLog(item);
    }
};
</script>
<style type="text/css">
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

/*html {
    overflow: hidden;
}

body {
    overflow: hidden;
    padding: 0;
    margin: 0;
    width: 100%;
    height: 100%;
    background: gray;
}

#log {
    background: white;
    margin: 0;
    padding: 0.5em 0.5em 0.5em 0.5em;
    position: absolute;
    top: 0.5em;
    left: 0.5em;
    right: 0.5em;
    bottom: 3em;
    overflow: auto;
}

#form {
    padding: 0 0.5em 0 0.5em;
    margin: 0;
    position: absolute;
    bottom: 1em;
    left: 0px;
    width: 100%;
    overflow: hidden;
}*/

</style>
</head>
<body>
<div id="log"></div>
<form id="form">
    <input type="submit" value="Send" />
    <input type="text" id="msg" size="64"/>
</form>
<ul id="selections">
{{range $k, $v := .Commands }}
 <li class="selection">
	<a class="play" data-sound="{{$k}}" href="#">{{$k}}</a>
	<audio preload="auto" src="{{$v}}" data-sound="{{$k}}">Your browser does not support the <code>audio</code> element.</audio>
 </li>
{{end}}
</ul>
</body>
</html>`))
