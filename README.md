# jukebox


## Setup

First, upload your sounds somewhere and ensure that `sounds/index.json` contains valid URLs (they may be root-relative if hosted on the same domain) for all of them. Note that this JSON file may have any name and does not need to be on the same domain as the sounds. Next, run as follows:

    go get github.com/gorilla/websocket
    go run -race main.go hub.go client.go -manifest http://localhost:8080/sounds/index.json


## Acknowledgments

Significant portions adapted (or used wholesale) from the Gorilla Websocket [chat example](https://github.com/gorilla/websocket/tree/master/examples/chat), with some inspiration from their other examples. Seriously, it took only a couple hours to integrate my existing project (which used polling) to use Websockets instead. Gorilla Web Toolkit rocks!