package main

import (
	"bytes"
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

var (
	isDown = NewAtomicInt(0)
	buffer = make(chan *http.Request, bufSize)
	client = http.Client{Timeout: 3 * time.Second}
)

func checkBackend() {
	setTo := int32(-1)
	resp, err := client.Get(backendCheck)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode < 300 {
			setTo = 0
		}
	}

	v := isDown.Value()
	if v != setTo {
		if setTo == 1 {
			log.Println("status changed to UP")
		} else {
			log.Println("status changed to DOWN")
		}
		isDown.Set(setTo)
	}
}

func rec() {
	r := recover()
	if r == nil {
		return
	}
	log.Printf("PANIC: %+v\n", r)
}

// AtomicInt uses sync/atomic to store and read the value of an int32.
type AtomicInt int32

// NewAtomicInt creates an new AtomicInt.
func NewAtomicInt(value int32) *AtomicInt {
	var i AtomicInt
	i.Set(value)
	return &i
}

func (i *AtomicInt) Set(value int32) { atomic.StoreInt32((*int32)(i), value) }
func (i *AtomicInt) Value() int32    { return atomic.LoadInt32((*int32)(i)) }

func main() {
	log.SetPrefix("httpbuf: ")

	defer rec()

	// Ping backend status.
	go func() {
		defer rec()
		for {
			time.Sleep(backendPingFrequency)
			checkBackend()
		}
	}()

	// Send buffered requests.
	go func() {
		defer rec()

		for {
			time.Sleep(bufferFrequency)

			if isDown.Value() == 0 {
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

				resp, err := client.Do(r)
				if err != nil {
					log.Printf("  Sending %s FAILED: %s\n", r.URL, err)
					buffer <- r
					continue
				}

				resp.Body.Close()
				if resp.StatusCode >= 300 {
					log.Printf("  Sending %s FAILED: %s\n", r.URL, resp.Status)
					buffer <- r
				} else {
					log.Printf("  Sending %s OKAY\n", r.URL)
				}
			}
		}
	}()

	// Collect all requests.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.Background()) // Clear context timeout.
		r.RequestURI = ""                       // Can't be set in requests.
		b, _ := ioutil.ReadAll(r.Body)          // Replace body so we can read it later.
		r.Body = ioutil.NopCloser(bytes.NewReader(b))

		log.Printf("buffering %s\n", r.URL)
		buffer <- r
		w.WriteHeader(http.StatusNoContent)
	})
	log.Printf("Ready on %s\n", listen)
	log.Fatal(http.ListenAndServe(listen, nil))
}
