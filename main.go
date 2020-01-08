package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"zgo.at/utils/syncutil"
	"zgo.at/utils/timeutil"
)

var (
	status     = syncutil.NewAtomicInt(0)
	lastChange = syncutil.NewAtomicInt(int32(time.Now().UTC().Unix()))
	buffer     = make(chan *http.Request, bufSize)
	client     = http.Client{Timeout: 3 * time.Second}
)

func checkBackend() {
	setTo := int32(0)
	resp, err := client.Get(backendCheck)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode < 300 {
			setTo = 1
		}
	}

	if status.Value() != setTo {
		status.Set(setTo)
		lastChange.Set(int32(time.Now().UTC().Unix())) // TODO: 2038
	}
}

func main() {
	defer rec()

	// Ping backend status.
	checkBackend()
	go func() {
		defer rec()
		for {
			checkBackend()
			time.Sleep(backendPingFrequency)
		}
	}()

	// Send buffered requests.
	go func() {
		defer rec()

		var i uint8
		for {
			time.Sleep(bufferFrequency)

			if i%24 == 0 {
				fmt.Println("Buffer        Backend    HeapAlloc    TotalAlloc       Live     Sys    NumGC")
				i = 0
			}
			i++

			change := time.Now().UTC().Sub(time.Unix(int64(lastChange.Value()), 0))
			st := fmt.Sprintf("%s down", timeutil.FormatDuration(change))
			s := false
			if status.Value() == 1 {
				s = true
				st = fmt.Sprintf("%s up", timeutil.FormatDuration(change))
			}

			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			fmt.Printf("%s    %s %s   %sK    %sK    %s    %sM    %s\n",
				fill(uint64(len(buffer)), 6),
				strings.Repeat(" ", 10-len(st)), st,
				fill(m.Alloc/1024, 9),
				fill(m.TotalAlloc/1024, 9),
				fill(m.Mallocs-m.Frees, 7),
				fill(m.Sys/1024/1014, 3),
				fill(uint64(m.NumGC), 5))

			if !s {
				continue
			}

			l := len(buffer)
			if l == 0 {
				continue
			}
			if l > backendBurst {
				l = backendBurst
			}

			for j := 0; j < l; j++ {
				r := <-buffer
				r.URL.Scheme = "http"
				if r.TLS != nil {
					r.URL.Scheme = "https"
				}
				r.URL.Host = r.Host

				fmt.Printf("  sending %s … ", r.URL)
				resp, err := client.Do(r)
				if err != nil {
					fmt.Printf("failed: %s\n", err)
					buffer <- r
					continue
				}

				resp.Body.Close()
				if resp.StatusCode >= 300 {
					fmt.Printf("failed: %s\n", resp.Status)
					buffer <- r
				} else {
					fmt.Println("Okay")
				}
			}
		}
	}()

	// Collect all requests.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Clear context timeout.
		r = r.WithContext(context.Background())

		// Can't be set in requests.
		r.RequestURI = ""

		// Replace body with bytes.Reader as we can't read the standard body
		// after getting it back from the channel.
		b, _ := ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewReader(b))

		buffer <- r
		w.WriteHeader(http.StatusNoContent)
	})

	fmt.Printf("Ready on %s\n\n", listen)
	log.Fatal(http.ListenAndServe(listen, nil))
}

func rec() {
	r := recover()
	if r == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "httpbuf: panic: %+v\n", r)
}

func fill(s uint64, n int) string {
	ss := fmt.Sprintf("%d", s)
	l := len(ss)
	if l >= n {
		return ss
	}
	return strings.Repeat(" ", n-l) + ss
}
