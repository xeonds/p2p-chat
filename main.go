package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jroimartin/gocui"
)

type Message struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"timestamp"`
	Msg       string `json:"msg"`
}

var (
	clientID    string
	clientPort  int
	messages    []Message
	allMessages = make(map[string]Message)
	clients     = make(map[string]string) // id -> ip:port
	upgrader    = websocket.Upgrader{}
	ui          *gocui.Gui
	messageView = "messages"
	clientsView = "clients"
	sendView    = "send"
	logView     = "log"
	logActive   bool
)

func main() {
	clientID = getLocalIP()
	clientPort = rand.Intn(10000) + 10000
	go startBroadcast()
	go startServer()

	g, err := gocui.NewGui(gocui.OutputNormal)
	if err != nil {
		log.Panicln(err)
	}
	defer g.Close()

	ui = g
	g.SetManagerFunc(layout)
	if err := keybindings(g); err != nil {
		log.Panicln(err)
	}

	if err := g.MainLoop(); err != nil && err != gocui.ErrQuit {
		log.Panicln(err)
	}
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatal(err)
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

func startBroadcast() {
	conn, err := net.Dial("udp", "255.255.255.255:9876")
	if err != nil {
		log.Fatal("Broadcast:", err)
	}
	defer conn.Close()

	for {
		message := fmt.Sprintf("%s:%d", clientID, clientPort)
		fmt.Fprint(conn, message)
		vlog("Broadcasting:", message)
		time.Sleep(2 * time.Second)
	}
}

func startServer() {
	http.HandleFunc("/ws", wsHandler)
	go http.ListenAndServe(":"+strconv.Itoa(clientPort), nil)

	udpAddr, err := net.ResolveUDPAddr("udp", ":9876")
	if err != nil {
		vlog(err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		vlog(err)
	}
	defer conn.Close()
	buf := make([]byte, 1024)

	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			vlog(err)
			continue
		}
		func(_data string, _addr *net.UDPAddr) {
			parts := strings.Split(_data, ":")
			if len(parts) != 2 {
				return
			}

			id := parts[0]
			ipPort := _addr.IP.String() + ":" + parts[1]
			// ensure the client isn't connected before
			if id != clientID {
				clients[id] = ipPort
				updateClientsView()
				go connectToClient(ipPort)
			}
		}(string(buf[:n]), addr)
	}
}

func connectToClient(ipPort string) {
	u := "ws://" + ipPort + "/ws"
	vlog("Connecting to:", u)
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		vlog("Dial:", err)
		return
	}
	defer conn.Close()

	var newMessages []Message
	err = conn.ReadJSON(&newMessages)
	if err != nil {
		vlog("ReadJSON:", err)
		return
	}

	for _, msg := range newMessages {
		allMessages[msg.ID] = msg
	}

	updateMessagesView()
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		vlog("Upgrade:", err)
		return
	}
	defer conn.Close()
	err = conn.WriteJSON(messages)
	if err != nil {
		vlog("WriteJSON:", err)
		return
	}
}

func layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()

	if v, err := g.SetView(messageView, 0, 0, maxX-1, maxY-6); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Messages"
		v.Autoscroll = true
		v.Wrap = true
	}

	if v, err := g.SetView(clientsView, 0, maxY-6, maxX/2-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Clients"
	}

	if v, err := g.SetView(sendView, maxX/2, maxY-6, maxX-1, maxY-1); err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
		v.Title = "Send"
		v.Editable = true
		if _, err := g.SetCurrentView(sendView); err != nil {
			return err
		}
	}

	// if v, err := g.SetView(logView, maxX/4, maxY/4, maxX-1-maxX/4, maxY-1-maxY/4); err != nil {
	// 	if err != gocui.ErrUnknownView {
	// 		return err
	// 	}
	// 	v.Frame = true
	// 	v.Wrap = true
	// 	v.Autoscroll = true
	// 	v.Title = "Log"
	// }

	return nil
}

func keybindings(g *gocui.Gui) error {
	if err := g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, quit); err != nil {
		return err
	}
	if err := g.SetKeybinding(sendView, gocui.KeyEnter, gocui.ModNone, sendMessage); err != nil {
		return err
	}
	if err := g.SetKeybinding(messageView, gocui.KeyPgup, gocui.ModNone, cursorPgup); err != nil {
		return err
	}
	if err := g.SetKeybinding(messageView, gocui.KeyPgdn, gocui.ModNone, cursorPgdn); err != nil {
		return err
	}
	if err := g.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, toggleLog); err != nil {
		return err
	}
	return nil
}

func sendMessage(g *gocui.Gui, v *gocui.View) error {
	msg := strings.TrimSpace(v.Buffer())
	v.Clear()
	v.SetCursor(0, 0)

	if msg == "" {
		return nil
	}

	ts := time.Now().Unix()
	newMsg := Message{
		ID:        fmt.Sprintln(clientID, ts),
		Timestamp: ts,
		Msg:       msg,
	}
	messages = append(messages, newMsg)
	allMessages[newMsg.ID] = newMsg
	updateMessagesView()
	return nil
}

func cursorPgup(g *gocui.Gui, v *gocui.View) error {
	ox, oy := v.Origin()
	_, sy := v.Size()
	if oy > sy {
		v.SetOrigin(ox, oy-sy)
	} else {
		v.SetOrigin(ox, 0)
	}
	return nil
}

func cursorPgdn(g *gocui.Gui, v *gocui.View) error {
	ox, oy := v.Origin()
	_, sy := v.Size()
	if oy+sy < len(messages) {
		v.SetOrigin(ox, oy+sy)
	}
	return nil
}

func quit(g *gocui.Gui, v *gocui.View) error {
	return gocui.ErrQuit
}

func updateMessagesView() {
	ui.Update(func(g *gocui.Gui) error {
		v, err := g.View(messageView)
		if err != nil {
			return err
		}
		v.Clear()
		for _, msg := range allMessages {
			fmt.Fprintf(v, "[%s] %s: %s\n", time.Unix(msg.Timestamp, 0).Format("15:04:05"), msg.ID, msg.Msg)
		}
		return nil
	})
}

func updateClientsView() {
	ui.Update(func(g *gocui.Gui) error {
		v, err := g.View(clientsView)
		if err != nil {
			return err
		}
		v.Clear()
		for id, ip := range clients {
			fmt.Fprintf(v, "%s: %s\n", id, ip)
		}
		return nil
	})
}

func toggleLog(g *gocui.Gui, v *gocui.View) error {
	logActive = !logActive
	if logActive {
		g.SetViewOnTop(logView)
	} else {
		g.SetViewOnBottom(logView)
	}
	return nil
}

func vlog(args ...interface{}) {
	if logActive {
		ui.Update(func(g *gocui.Gui) error {
			v, err := g.View(logView)
			if err != nil {
				return err
			}
			fmt.Fprintln(v, args...)
			return nil
		})
	}
}
