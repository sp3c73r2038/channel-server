
```bash
$ make build
$ bin/channel-server -addr 127.0.0.1:7788
```

chat-like server in specific channel `ws://example.com/channel/${channelName}`

recv client example

```python
# -*- coding: utf-8 -*-
import websocket

conn = websocket.create_connection('ws://localhost:7788/channel/1')
while True:
    m = conn.recv()
    print(m)
```

send client example

```python
# -*- coding: utf-8 -*-
from datetime import datetime
import time

import websocket


conn = websocket.create_connection('ws://127.0.0.1:7788/channel/1')
while True:
    conn.send(str(datetime.now()))
    time.sleep(1)
```

use server as library

```go
package main

import (
	"flag"
	"log"
	"net/http"

	"channel-server/pkg/server"
)

var addr = flag.String("addr", ":8080", "http service address")

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
	http.ServeFile(w, r, "home.html")
}

func main() {
	flag.Parse()

	basePath := "/"

	s, err := server.NewServer(basePath)
	if err != nil {
		log.Fatal(err)
	}
	s.Mux.HandleFunc(basePath, serveHome)
	err = s.Serve(*addr)
	if err != nil {
		log.Fatal(err)
	}
}
```
