package main

import "time"

var (
	listen               = ":8100"                              // Listen address.
	bufSize              = 100_000                              // Channel buffer size.
	backendCheck         = "https://www.goatcounter.com/status" // Backend check GET, e.g. https://domain/status.json
	backendPingFrequency = 1 * time.Second                      // Check backend every second.
	bufferFrequency      = 10 * time.Second                     // Send buffer to backend every 10 seconds.
	backendBurst         = 10                                   // Max. burst of requests to send to backend.
)
