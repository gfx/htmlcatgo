package main

import "io"
import "net"
import "net/http"
import "log"
import "os"
import osexec "os/exec"
import "bufio"
import "fmt"
import "html/template"
import "flag"
import "math/rand"
import "strconv"
import "io/ioutil"

type Client struct {
	out  chan string
	quit chan bool
}

type Broadcaster struct {
	addClient    chan (*Client)
	removeClient chan (*Client)
	running      chan bool
}

func NewBroadcaster(stream *bufio.Reader) *Broadcaster {
	running := make(chan bool, 1)
	running <- true
	broadcaster := &Broadcaster{make(chan (*Client)), make(chan (*Client)), running}

	out := make(chan string)
	quit := make(chan bool)
	go func() {
		for {
			line, err := stream.ReadString('\n')
			if err != nil {
				log.Print(err)
				quit <- true
				return
			}
			out <- line
		}
	}()

	go func() {
		clients := [](*Client){}
		for {
			select {
			case line := <-out:
				for _, client := range clients {
					client.out <- line
				}
			case <-quit:
				<-broadcaster.running
				for _, client := range clients {
					client.quit <- true
				}
				close(broadcaster.running)
				return
			case c := <-broadcaster.addClient:
				clients = append(clients, c)
				broadcaster.running <- true
			case c := <-broadcaster.removeClient:
				newClients := make([](*Client), 0, len(clients))
				for _, ch := range clients {
					if ch != c {
						newClients = append(newClients, ch)
					}
				}
				clients = newClients
				broadcaster.running <- true
			}
		}
	}()

	return broadcaster
}

func (broadcaster *Broadcaster) AddClient(client *Client) bool {
	_, ok := <-broadcaster.running
	if !ok {
		return false
	}

	broadcaster.addClient <- client

	return true
}

func (broadcaster *Broadcaster) RemoveClient(client *Client) bool {
	_, ok := <-broadcaster.running
	if !ok {
		return false
	}

	broadcaster.removeClient <- client

	return true
}

func handleStream(res http.ResponseWriter, req *http.Request, broadcaster *Broadcaster) {
	f, ok := res.(http.Flusher)
	if !ok {
		http.Error(res, "Streaming unsupported", http.StatusInternalServerError)
		return
	}
	c, ok := res.(http.CloseNotifier)
	if !ok {
		http.Error(res, "Close notification unsupported", http.StatusInternalServerError)
		return
	}

	closer := c.CloseNotify()

	headers := res.Header()
	headers.Set("Content-Type", "text/event-stream; charset=utf-8")
	headers.Set("Cache-Control", "no-cache")

	client := &Client{out: make(chan string), quit: make(chan bool)}
	if !broadcaster.AddClient(client) {
		return
	}

	for {
		select {
		case line := <-client.out:
			fmt.Fprintf(res, "data:%s\n", line)
			fmt.Fprint(res, "\n")
			f.Flush()
		case <-client.quit:
			return
		case <-closer:
			broadcaster.RemoveClient(client)
			return
		}
	}
}

func emptyPort() int {
	port, err := strconv.Atoi(os.Getenv("HTTPCAT_PORT"))
	if err != nil {
		port = rand.Intn(1000) + 45192
	}

	for ; port < 60000; port += 1 {
		addr, err := net.ResolveTCPAddr("tcp4", fmt.Sprintf("localhost:%d", port))
		listener, err := net.ListenTCP("tcp4", addr)
		listener.Close()
		if err == nil {
			return port
		}
	}

	log.Fatal("Could not find empty port")
	return 0
}

func main() {
	port := flag.Int("port", emptyPort(), "port to bind (default 8080)")
	host := flag.String("host", "localhost", "url host (default localhost)")
	exec := flag.String("exec", "", "command to run passing htmlcatgot URL (default \"\")")
	flag.Parse()

	broadcaster := NewBroadcaster(bufio.NewReader(os.Stdin))

	http.HandleFunc("/stream", func(res http.ResponseWriter, req *http.Request) {
		handleStream(res, req, broadcaster)
	})

	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		executeIndexTemplate(res)
	})

	if len(*exec) == 0 {
		log.Printf("%s: http://%s:%d\n", os.Args[0], *host, *port)
	} else {
		go func() {
			cmd := osexec.Command(*exec, fmt.Sprintf("http://%s:%d", *host, *port))
			err := cmd.Run()
			if err != nil {
				log.Fatal(err)
			}
		}()
	}
	err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil)
	if err != nil {
		log.Fatal(err)
	}
}

func executeIndexTemplate(out io.Writer) {
	t, err := ioutil.ReadFile("template/main.html")
	if err != nil {
		log.Fatal(err)
	}
	var indexTemplate = template.Must(template.New("index").Parse(string(t)))
	indexTemplate.Execute(out, nil)
}
