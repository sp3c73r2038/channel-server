// Copyright 2013 The Gorilla WebSocket Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"flag"
	"net/http"

	"channel-server/pkg/chat"
)

func serveHome(w http.ResponseWriter, r *http.Request) {
	chat.LOGGER.Debug(r.URL)
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

	var addr = flag.String("addr", ":8080", "http service address")
	var debug = flag.Bool("debug", false, "debug logging")

	flag.Parse()

	chat.BuildLogger(*debug)

	mux := http.NewServeMux()
	mux.HandleFunc("/", serveHome)
	s := chat.NewServer(mux, "/channel/")
	err := s.Serve(*addr)
	if err != nil {
		panic(err)
	}
}
