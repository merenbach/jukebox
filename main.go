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
// TODO: replace Track nomenclature
// TODO: revamp fault tolerance (invalid sound, etc.)
// TODO: better log/history display in browser, plus status messages about joins/leaves--and don't try to play those...
// TODO: Lambda to run? Accept URI for sound library...
// TODO: different rooms, namespaced to allow multiple "conversations"
// TODO: Slack integration
// TODO: dedicated client app to submit?
// NOTE: portions based heavily on https://github.com/gorilla/websocket/tree/master/examples/chat

package main

import (
	"encoding/json"
	"flag"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"path"
)

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
/*func ServePlaylist(library map[string]string) {

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
}*/

var addr = flag.String("addr", "localhost:8080", "http service address")

//var addr = flag.String("addr", ":8080", "http service address")

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

var playlistLibrary map[string]string

func main() {
	bb, err := ioutil.ReadFile("sounds.json")
	if err != nil {
		log.Fatal(err)
	}

	json.Unmarshal(bb, &playlistLibrary)
	log.Println("Initializing with the following resources:", playlistLibrary)

	flag.Parse()
	hub := newHub()
	go hub.run()
	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})
	// TODO: improve this....
	http.HandleFunc("/play/", func(w http.ResponseWriter, r *http.Request) {
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
	fs := http.FileServer(http.Dir("sounds"))
	http.Handle("/sounds/", http.StripPrefix("/sounds/", fs))
	// <<<<<----
	err = http.ListenAndServe(*addr, nil)
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
	homeTemplate.Execute(w, struct {
		Library map[string]string
	}{
		//Url:      "ws://" + r.Host + "/ws",
		Library: playlistLibrary,
	})
}

var homeTemplate = template.Must(template.New("").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<title>Sound Machine</title>
<script type="text/javascript">
window.onload = function () {
    var conn;
    //var msg = document.getElementById("msg");
    var log = document.getElementById("log");

    function appendLog(item) {
        //var doScroll = log.scrollTop > log.scrollHeight - log.clientHeight - 1;
        log.appendChild(item);
        /*if (doScroll) {
            log.scrollTop = log.scrollHeight - log.clientHeight;
        }*/
    }

    /*document.getElementById("form").onsubmit = function () {
        if (!conn) {
            return false;
        }
        if (!msg.value) {
            return false;
        }
        conn.send(msg.value);
        msg.value = "";
        return false;
    };*/
	
	////>>
	Array.from(document.getElementsByClassName("play")).forEach( (e) => e.onclick = function(event) {
		event.preventDefault();
		if (!conn) {
			return false;
		}
		console.log("SEND: " + e.dataset.sound);
		conn.send(e.dataset.sound);
		return false;
	});
	////<<


	var PLAY_QUEUE = [];
	var PLAYING = false;

	window.setInterval(function() {
		if (!PLAYING && PLAY_QUEUE.length > 0) {
			var NEXT_UP = PLAY_QUEUE.shift();
			console.log("Okay, NEXT UP is: " + NEXT_UP);
			const selections = document.getElementById('selections');
			var audio = selections.querySelector('audio[data-sound="' + NEXT_UP + '"]');
			if (audio != null) {
				audio.onplay = function() {
					PLAYING = true;
				}
				audio.onended = function() {
					PLAYING = false;
				}
				audio.play();
			}
		}
	}, 100);
	
    if (window["WebSocket"]) {
		conn = new WebSocket("ws://" + document.location.host + "/ws");

        conn.onclose = function (evt) {
            var item = document.createElement("div");
            item.innerHTML = "<b>Connection closed.</b>";
            appendLog(item);
        };
        conn.onmessage = function (evt) {
			var messages = evt.data.split('\n');

            for (var i = 0; i < messages.length; i++) {

				var rsrc = messages[i];
				PLAY_QUEUE.push(rsrc);
				
			// const selections = document.getElementById('selections');
			// var audio = selections.querySelector('audio[data-sound="' + rsrc + '"]');
			/*if (audio == null) {
				audio = new Audio(rsrc);
				sounds.appendChild(audio);
			}*/
			// audio.play();
			/*try {
			audio.play();
			} catch (err) {
				console.log("Could not play sound: " + err)
			}*/

                var item = document.createElement("div");
                item.innerText = messages[i];
                appendLog(item);
            }
        };
    } else {
        var item = document.createElement("div");
        item.innerHTML = "<b>Your browser does not support WebSockets.</b>";
        appendLog(item);
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
}*/

</style>
</head>
<body>
<h1>Sound Machine</h1>
<p>Click on a sound below to play it for all connected clients!</p>
<ul id="selections">
{{range $k, $v := .Library}}
  <li class="selection">
	<a class="play" data-sound="{{$k}}" href="#">{{$k}}</a>
	<audio preload="auto" src="{{$v}}" data-sound="{{$k}}">Your browser does not support the <code>audio</code> element.</audio>
  </li>
{{end}}
</ul>
<div id="log"></div>
</body>
</html>`))
