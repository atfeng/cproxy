package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

func main() {
	go refreshContainers()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		hostname, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			hostname = r.Host
		}
		log.Printf("New request to host %s, hostname=%s", r.Host, hostname)

		targetUrl, ok := containerTargets.Load(hostname)
		if !ok {
			http.Error(w, "Target not found", http.StatusNotFound)
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

var containerTargets = sync.Map{}

func refreshContainers() {
	c := time.Tick(5 * time.Second)
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
