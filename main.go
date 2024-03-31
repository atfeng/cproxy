package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

var updateInterval = 5 * time.Second // Interval for container update
var domainSuffix string

func main() {
	if suffix, found := os.LookupEnv("DOMAIN_SUFFIX"); found {
		if len(suffix) < 2 || !strings.HasPrefix(suffix, ".") {
			log.Fatal("DOMAIN_SUFFIX should like .test.com")
		}
		domainSuffix = suffix
	}
	go refreshContainers()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hostname, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			hostname = r.Host
		}
		log.Printf("New request to host %s, hostname=%s", r.Host, hostname)

		targetUrl, ok := containerTargets.Load(hostname)
		if !ok {
			w.Write(generateIndex(domainSuffix))
			return
		}

		proxy := httputil.NewSingleHostReverseProxy(targetUrl.(*url.URL))
		proxy.ServeHTTP(w, r)
	})

	err := http.ListenAndServe(":60600", nil)
	if err != nil {
		log.Fatal(err)
	}
}

func generateIndex(domainSuffix string) []byte {
	lists := make([]string, 0, 0)
	containerTargets.Range(func(k, v any) bool {
		name := k.(string)

		domain := name + domainSuffix
		list := "\t\t" + fmt.Sprintf(`<li><a href="%s" target="_blank">%s</a></li>`, domain, domain)
		lists = append(lists, list)
		return true
	})

	html := "<html>\n<body>\n"
	if len(lists) > 0 {
		sort.Strings(lists)
		html += "\t<ul>\n" + strings.Join(lists, "\n") + "\n\t</ul>\n"
	} else {
		html += "\t<h1>No Container running</h1>\n"
	}

	html += "</body>\n</html>"
	return []byte(html)
}

var containerTargets = sync.Map{}

func refreshContainers() {
	c := time.Tick(updateInterval)
	for range c {
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			log.Printf("Fail to create docker container: %s", err)
			continue
		}

		containers, err := cli.ContainerList(context.Background(), containerTypes.ListOptions{})
		if err != nil {
			log.Printf("Fail to fetch container list: %s", err)
			continue
		}

		targets := make(map[string]*url.URL, len(containers))
		for _, container := range containers {
			if len(container.Names) != 1 {
				log.Printf("Invalid container names: %s", container.Names)
				continue
			}

			name := strings.TrimPrefix(container.Names[0], "/")

			var ipAddress string
			for netType, setting := range container.NetworkSettings.Networks {
				//TODO: Add support for more network types
				if netType != "bridge" {
					continue
				}
				ipAddress = setting.IPAddress
			}

			if ipAddress == "" {
				log.Printf("Bridge network ip not found for %s", name)
				continue
			}

			var privatePort uint16
			for _, port := range container.Ports {
				privatePort = port.PrivatePort
			}
			if privatePort == 0 {
				log.Printf("Expose port not found for %s", name)
				continue
			}

			targetUrl := &url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s:%d", ipAddress, privatePort),
			}

			targets[name] = targetUrl

		}

		containerTargets.Range(func(k, v any) bool {
			name := k.(string)
			oldVal := v.(*url.URL)
			_, found := targets[name]
			if !found {
				log.Printf("Remove rule %s=>%s", name, oldVal)
				containerTargets.Delete(k)
			}

			return true
		})

		for name, targetUrl := range targets {
			log.Printf("Update rule %s=>%s", name, targetUrl)
			containerTargets.Store(name, targetUrl)
		}
	}
}
