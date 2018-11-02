package main

import (
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"

	consul "github.com/hashicorp/consul/api"
)

// define a hard-coded HTML template (just so we don't have to distribute it separately)
const TEMPLATE = `
<!DOCTYPE html>
<html>
	<head>
		<style type="text/css">
			.container {
				display: flex;
				align-items: center;
				justify-content: center;
				flex-direction: column;
			}

			li {
				list-style: none;
				padding: 5px;
				word-spacing: 10px;
			}

			li.current {
				color: green;
				list-style: disclosure-closed;
			}
		</style>
	</head>
	<body>
		<div class="container">
			<h4>Web Nodes</h4>
			<ul>
			{{range .WebNodes}}
				{{if .Current}}
					<li class="current">{{.Name}}   -   {{.Address}}</li>
				{{else}}
					<li>{{.Name}}   -   {{.Address}}</li>
				{{end}}
			{{end}}
			</ul>
			<h4>Other Nodes</h4>
			<ul>
			{{range .OtherNodes}}
				<li>{{.Name}}   -   {{.Address}}</li>
			{{end}}
			</ul>
		</div>
	</body>
</html>`

var dc string
var iface string
var NO_CIDR = regexp.MustCompile("^([0-9.]+)/[0-9]+$")

// GetInterfaceIP returns the primary IPv4 address of the given interface
func GetInterfaceIP(face string) (string, bool) {
	info, _ := net.InterfaceByName(iface)
	addrs, _ := info.Addrs()
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
			return NO_CIDR.ReplaceAllString(ipnet.String(), "${1}"), true
		}
	}
	return "", false
}

// Node is just a condensed version of consul.Node, with some additional data for the page data
type Node struct {
	Name    string
	Address string
	Current bool
}

// PageData holds all nodes for the template rendering
type PageData struct {
	WebNodes   []Node
	OtherNodes []Node
}

func handler(w http.ResponseWriter, r *http.Request) {
	// get the IP for the interface
	ip, ok := GetInterfaceIP(iface)
	if !ok {
		fmt.Fprint(w, "Failed to get local IP\n")
		return
	}

	// prepare a configuration to connect to the Consul client
	config := consul.DefaultConfig()
	config.Address = ip + ":8500"
	config.Datacenter = dc

	// connect to the Consul client
	client, err := consul.NewClient(config)
	if err != nil {
		fmt.Fprint(w, "Failed to connect to Consul\n")
		return
	}

	// get a list of all nodes Consul knows about
	nodes, _, err := client.Catalog().Nodes(nil)
	if err != nil {
		fmt.Fprint(w, "Failed to get Consul nodes\n")
		return
	}

	// iterate through all the nodes
	var data PageData
	for _, node := range nodes {
		// separate the nodes by name prefix
		if strings.HasPrefix(node.Node, "web") {
			// append web nodes to its list
			data.WebNodes = append(data.WebNodes, Node{
				Name:    node.Node,
				Address: node.Address,
				Current: node.Address == ip,
			})
		} else {
			// append all other nodes to its list
			data.OtherNodes = append(data.OtherNodes, Node{
				Name:    node.Node,
				Address: node.Address,
			})
		}
	}

	// generate a template
	tpl, err := template.New("webpage").Parse(TEMPLATE)
	if err != nil {
		fmt.Fprint(w, "Failed to render template\n")
	} else {
		// render the template as a response
		tpl.Execute(w, data)
	}
}

func main() {
	// get the interface name from the environment
	iface = os.Getenv("IFACE")
	if iface == "" {
		log.Fatal("Missing environment variable 'IFACE'")
	} else if _, err := net.InterfaceByName(iface); err != nil {
		log.Fatalf("Interface '%s' doesn't exist\n", iface)
	}

	// get the datacenter name from the environment
	dc = os.Getenv("DATACENTER")
	if dc == "" {
		log.Fatal("Missing environment variable 'DATACENTER'")
	}

	// get the port from the environment, with a default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// register consul health check endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "OK")
	})

	// serve the handler
	http.HandleFunc("/", handler)
	log.Println("Serving on port " + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
