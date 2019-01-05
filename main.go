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
// TODO: allow refreshing of list if remote manifest updated??

package main

import (
	"flag"
	"html/template"
	"log"
	"net/http"
	"path"
	"strings"
)

var addr = flag.String("addr", "localhost:8080", "http service address")
var manifest = flag.String("manifest", "", "URL of sound library JSON")

// // GetRemoteFile reads the contents of a file from a remote URL.
// func getRemoteFile(url string) ([]byte, error) {
// 	resp, err := http.Get(url)
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	defer func() { _ = resp.Body.Close() }()
// 	return ioutil.ReadAll(resp.Body)
//}

func main() {
	flag.Parse()
	hub := newHub()
	go hub.run()

	log.Println("Initializing with address: ", *addr)
	log.Println("Initializing with manifest: ", *manifest)
	if strings.HasPrefix(*manifest, "/") {
		*manifest = path.Join(*addr, *manifest)
		log.Println("Adjusting manifest to: ", *manifest)
	}

	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWs(hub, w, r)
	})
	// TODO: improve this....
	http.HandleFunc("/play/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			http.Redirect(w, r, *manifest, http.StatusSeeOther)
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
	fs := http.FileServer(http.Dir("sounds"))
	http.Handle("/sounds/", http.StripPrefix("/sounds/", fs))
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
	homeTemplate.Execute(w, "ws://"+r.Host+"/ws")
}

var homeTemplate = template.Must(template.New("").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<title>Sound Machine</title>
<script type="text/javascript">
(function() {
"use strict";
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

	var audioElements = {};

	fetch('/play/')
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
	   	.then(function(obj) {
			const sounds = document.getElementById("sounds");
			Object.entries(obj).forEach(
				([key, value]) => {
					console.log(key, value);
					const audio = new Audio(value);
					audio.preload = 'auto';
					sounds.appendChild(audio);

					audioElements[key] = audio;

					const button = document.createElement('a');
					button.href = '#';
					button.innerHTML = key;
					sounds.appendChild(button);
					button.onclick = function(event) {
						event.preventDefault();
						if (!conn) {
							return false;
						}

						console.log("SEND: " + key);
						conn.send(key);
						return false;
					};
				}
			);
	    });

	var player = function() {
		var currentTrack = false;
		var queue = []; // TODO: const?

		function append(t) {
			queue.push(t);
		}
		function next() {
			if (!currentTrack && queue.length > 0) {
				const nextTrack = queue.shift();
				console.log("PLAY: " + nextTrack);
				const audio = audioElements[nextTrack];
				audio.onplay = function() {
					currentTrack = audio;
				}
				audio.onended = function() {
					currentTrack = false;
				}
				audio.play();
			}
		}

		/*function stop() {
			if (currentTrack) {
				currentTrack.pause();
				currentTrack.currentTime = 0;
				currentTrack = false;
			}
		}*/
		
		window.setInterval(function() {
			next();
		}, 100);

		return {
			append: append,
		};
	}();
	var queueTrack = player.append;
	
    if (window["WebSocket"]) {
		conn = new WebSocket("{{.}}");

        conn.onclose = function (evt) {
            var item = document.createElement("div");
            item.innerHTML = "<b>Connection closed.</b>";
            appendLog(item);
        };
        conn.onmessage = function (evt) {
			var messages = evt.data.split('\n');

            for (var i = 0; i < messages.length; i++) {
				queueTrack(messages[i]);
				
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
}());
</script>
<style type="text/css">
body {
	background-color: #333;
	color: #ccc;
}

#sounds {
	padding-left: 0;
}

#sounds a {
	display: inline-block;
	padding: .25em .5em;
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
<div id="sounds"></div>
<div id="log"></div>
</body>
</html>`))
