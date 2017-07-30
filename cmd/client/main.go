package main

import (
	"bytes"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/blackss2/devfarm/pkg/packer"

	"golang.org/x/net/websocket"
)

const (
	gHostAddr = "115.68.218.153"
)

func main() {
	Command := os.Args[1]
	BuildFlags := []string{}
	if len(os.Args) > 3 {
		BuildFlags = os.Args[2 : len(os.Args)-1]
	}
	Packages := os.Args[len(os.Args)-1]
	/*
		Command := "install"
		BuildFlags := []string{"-v", "-gcflags", "-N -l"}
		Packages := "github.com/blackss2/devfarm/cmd/intest"
	*/

	data, err := packer.PackSourceZip(Command, BuildFlags, Packages)
	if err != nil {
		panic(err)
	}

	res, err := http.Post("http://"+gHostAddr+"/api/spaces", "application/octet-stream", bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}
	if res.StatusCode != 200 {
		panic(string(body))
	}

	Id := string(body)
	if len(Id) == 0 {
		panic("empty id")
	}

	pc := NewPortContext()

	if true {
		ws, err := websocket.Dial("ws://"+gHostAddr+"/api/spaces/"+Id+"/stdout", "", "http://"+gHostAddr+"/")
		if err != nil {
			panic(err)
		}

		go func() {
			for {
				msg := ""
				err := websocket.Message.Receive(ws, &msg)
				if err != nil {
					os.Exit(0)
					return
				}
				os.Stdout.WriteString(msg)
			}
		}()
	}
	if true {
		ws, err := websocket.Dial("ws://"+gHostAddr+"/api/spaces/"+Id+"/stderr", "", "http://"+gHostAddr+"/")
		if err != nil {
			panic(err)
		}

		go func() {
			for {
				msg := ""
				err := websocket.Message.Receive(ws, &msg)
				if err != nil {
					os.Exit(0)
					return
				}
				os.Stderr.WriteString(msg)
			}
		}()
	}
	if true {
		ws, err := websocket.Dial("ws://"+gHostAddr+"/api/spaces/"+Id+"/portchan", "", "http://"+gHostAddr+"/")
		if err != nil {
			panic(err)
		}

		go func() {
			for {
				msg := ""
				err := websocket.Message.Receive(ws, &msg)
				if err != nil {
					os.Exit(0)
					return
				}
				list := strings.Split(msg, ",")
				ports := make([]string, 0, len(list))
				for _, v := range list {
					if len(v) > 0 {
						ports = append(ports, v)
					}
				}
				pc.UpdateListenPorts(ports)
			}
		}()
	}
	if true {
		ws, err := websocket.Dial("ws://"+gHostAddr+"/api/spaces/"+Id+"/stdin", "", "http://"+gHostAddr+"/")
		if err != nil {
			panic(err)
		}

		//go func() {
		func() {
			msg := make([]byte, 1000)
			for {
				n, err := os.Stdin.Read(msg)
				if err != nil {
					os.Exit(0)
					return
				}

				err = websocket.Message.Send(ws, msg[:n])
				if err != nil {
					os.Exit(0)
					return
				}
			}
		}()
	}
}

type PortContext struct {
	sync.Mutex
	portHash   map[string]bool
	listenHash map[string]net.Listener
}

func NewPortContext() *PortContext {
	pc := &PortContext{
		portHash:   make(map[string]bool),
		listenHash: make(map[string]net.Listener),
	}
	return pc
}

func (pc *PortContext) UpdateListenPorts(Ports []string) {
	pc.Lock()
	defer pc.Unlock()

	nhash := make(map[string]bool)
	for k, v := range pc.portHash {
		nhash[k] = v
	}
	for _, v := range Ports {
		delete(nhash, v)
	}
	for _, v := range Ports {
		if !pc.portHash[v] {
			pc.SetupListen(v)
			pc.portHash[v] = true
		}
	}
	for k, _ := range nhash {
		if l, has := pc.listenHash[k]; has {
			l.Close()
			delete(pc.listenHash, k)
		}
	}
}

func (pc *PortContext) SetupListen(Port string) {
	// Listen on TCP port 2000 on all interfaces.
	l, err := net.Listen("tcp", ":"+Port)
	if err != nil {
		return
	}
	defer l.Close()

	pc.listenHash[Port] = l
	for {
		// Wait for a connection.
		local, err := l.Accept()
		if err != nil {
			return
		}

		remote, err := net.Dial("tcp", gHostAddr+":"+Port)
		if err != nil {
			return
		}

		go func(a net.Conn, b net.Conn) {
			go func() {
				defer a.Close()
				defer b.Close()

				msg := make([]byte, 1000)
				for {
					n, err := a.Read(msg)
					if err != nil {
						return
					}
					_, err = b.Write(msg[:n])
					if err != nil {
						return
					}
				}
			}()
			func() {
				defer a.Close()
				defer b.Close()

				msg := make([]byte, 1000)
				for {
					n, err := b.Read(msg)
					if err != nil {
						return
					}
					_, err = a.Write(msg[:n])
					if err != nil {
						return
					}
				}
			}()
		}(local, remote)
	}
}
