# jukebox

## Setup

First, upload your sounds somewhere and ensure that `sounds/index.json` contains valid URLs (they may be root-relative if hosted on the same domain) for all of them. Note that this JSON file may have any name and does not need to be on the same domain as the sounds. Next, run as follows:

    go get github.com/gorilla/websocket
    go run -race main.go hub.go client.go -sounds http://example.com/sounds/index.json