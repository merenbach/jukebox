//(function() {
"use strict";
window.onload = function () {
const launch = document.getElementById("launch");
  launch.onclick = function() {
	  launch.style.display = 'none';
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
console.log("audio elements = " + JSON.stringify(audioElements));
console.log(audioElements);
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
		conn = new WebSocket("ws://" + document.location.host + "/ws");

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
};
//}());