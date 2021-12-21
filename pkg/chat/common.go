package chat

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var LOGGER *zap.SugaredLogger
var log *zap.SugaredLogger

func BuildLogger(debug bool) {

	lvl := zap.InfoLevel
	if debug {
		lvl = zap.DebugLevel
	}

	config := zap.Config{
		Level:            zap.NewAtomicLevelAt(lvl),
		Development:      true,
		Encoding:         "console",
		EncoderConfig:    zap.NewDevelopmentEncoderConfig(),
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, err := config.Build()
	if err != nil {
		panic(err)
	}
	log = logger.Sugar()
	LOGGER = log
}

const (
	MaxMsgSize = 1024 * 4
)

var (
	NewLine = []byte{'\n'}
	Space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Config struct {
	WriteWait  time.Duration
	PongWiat   time.Duration
	MaxMsgSize int
}

type Client struct {
	Channel   *Channel
	Conn      *websocket.Conn
	Cancel    context.CancelFunc
	Heartbeat time.Duration
	Send      chan []byte
}

func (c *Client) WithHeartbeat(h time.Duration) *Client {
	c.Heartbeat = h
	return c
}

type Hub struct {
	Register   chan *Client
	Unregister chan *Client
	Channels   map[string]*Channel

	ChLock sync.Mutex
}

type Channel struct {
	Broadcast chan []byte
	Clients   map[*Client]bool
	Hub       *Hub
}

func (c *Channel) AddClient(client *Client) {
	ctx, cancel := context.WithCancel(context.Background())
	client.Cancel = cancel
	c.Clients[client] = true

	// setup client connection
	client.Conn.SetReadLimit(MaxMsgSize)

	if client.Heartbeat > 0 {
		// setup server side ping/pong handler
		// refresh the socket deadline
		client.Conn.SetPingHandler(func(data string) error {
			log.Debugf("got ping from %s", client.Conn.RemoteAddr())
			deadline := time.Now().Add(client.Heartbeat)
			client.Conn.SetReadDeadline(deadline)
			client.Conn.SetWriteDeadline(deadline)
			return client.Conn.WriteMessage(websocket.PongMessage, []byte(data))
		})
		client.Conn.SetPongHandler(func(data string) error {
			log.Debugf("got pong from %s", client.Conn.RemoteAddr())
			deadline := time.Now().Add(client.Heartbeat)
			client.Conn.SetReadDeadline(deadline)
			client.Conn.SetWriteDeadline(deadline)
			return nil
		})
		go heartbeat(ctx, client)
	}

	// start goroutine
	go inbound(ctx, client)
	go outbound(ctx, client)
	log.Infof("%s has connected", client.Conn.RemoteAddr())
}

func (c *Channel) RemoveClient(client *Client) {
	// stop goroutine
	client.Cancel()

	if _, ok := c.Clients[client]; ok {
		log.Infof("%s has disconnected", client.Conn.RemoteAddr())
	}

	// remove client
	delete(c.Clients, client)
	// close connection
	client.Conn.Close()
}

func (c *Channel) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-c.Broadcast:
			for client := range c.Clients {
				client.Send <- msg
			}
		}
	}
}

// client -> chat-server
func inbound(parent context.Context, client *Client) {
	defer func() {
		client.Channel.Hub.Unregister <- client
	}()
	for {
		select {
		case <-parent.Done():
			return
		default:
			_, msg, err := client.Conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(
					err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Warnf("error: %v", err)
				}
				return
			}
			msg = bytes.TrimSpace(bytes.Trim(msg, "\x00"))
			client.Channel.Broadcast <- msg
		}
	}
}

// chat-server -> client
func outbound(parent context.Context, client *Client) {
	defer func() {
		client.Channel.Hub.Unregister <- client
	}()
	for {
		select {
		case msg, ok := <-client.Send:
			if !ok {
				// client has closed
				client.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := client.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(msg)

			// write queued message
			n := len(client.Send)
			for i := 0; i < n; i++ {
				w.Write(NewLine)
				w.Write(<-client.Send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-parent.Done():
			return
		}
	}

}

// chat-server -> client heartbeat
func heartbeat(parent context.Context, client *Client) {
	interval := (client.Heartbeat * 9) / 10
	ticker := time.NewTicker(interval)
	defer func() {
		ticker.Stop()
	}()

	for {
		select {
		case <-ticker.C:
			log.Debugf("send ping to %s", client.Conn.RemoteAddr())
			err := client.Conn.WriteMessage(websocket.PingMessage, []byte{})
			if err != nil {
				return
			}
		case <-parent.Done():

		}
	}
}

func NewHub() *Hub {
	return &Hub{
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Channels:   make(map[string]*Channel),
		ChLock:     sync.Mutex{},
	}
}

func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case client := <-h.Register:
			client.Channel.AddClient(client)
		case client := <-h.Unregister:
			client.Channel.RemoveClient(client)
		case <-ctx.Done():
			return
		}
	}
}

func (h *Hub) GetChannel(name string) *Channel {
	h.ChLock.Lock()
	defer h.ChLock.Unlock()

	if ch, ok := h.Channels[name]; ok {
		return ch
	}

	rv := &Channel{
		Broadcast: make(chan []byte),
		Clients:   make(map[*Client]bool),
		Hub:       h,
	}
	h.Channels[name] = rv
	// FIXME:
	go rv.Run(context.Background())
	return rv
}

func NewClient(ch *Channel, conn *websocket.Conn) *Client {
	return &Client{
		Channel: ch,
		Conn:    conn,
		Send:    make(chan []byte, 256),
	}
}

func ServeChannel(
	hub *Hub,
	basePath string) func(http.ResponseWriter, *http.Request) {

	prefix := strings.TrimRight(basePath, "/")

	return func(w http.ResponseWriter, r *http.Request) {

		channel := strings.TrimPrefix(r.URL.Path, prefix)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error(err)
			return
		}
		ch := hub.GetChannel(channel)
		client := NewClient(ch, conn)

		var heartbeat time.Duration
		hb := r.URL.Query().Get("heartbeat")
		if len(hb) > 0 {
			h, err := strconv.Atoi(hb)
			if err != nil {
				log.Error(err)
				return
			}
			heartbeat = time.Duration(h) * time.Second
		}

		if heartbeat > 0 {
			log.Infof("client with heartbeat: %s", heartbeat)
			client = client.WithHeartbeat(heartbeat)
		}

		hub.Register <- client
	}
}

type ChatServer struct {
	Mux *http.ServeMux
	Hub *Hub
}

func (s *ChatServer) Serve(addr string) error {
	log.Infof("ChatServer serving at %s", addr)
	return http.ListenAndServe(addr, s.Mux)
}

func NewServer(mux *http.ServeMux, basePath string) (rv *ChatServer) {
	hub := NewHub()
	go hub.Run(context.Background())

	rv = &ChatServer{
		Hub: hub,
		Mux: mux,
	}

	mux.HandleFunc(basePath, ServeChannel(hub, basePath))
	return rv
}
