Buffer HTTP requests when a backend is down, and send them when it's back up.

Use case: for goatcounter.com users not being able to view stats for a few hours
is not great but not a drama either. Missing all stats for a few hours is more
of a problem.

Use this in combination with a load balancer, which sends requests to it when
the main server is down. For example in Varnish:

    sub vcl_backend_error {
        if (bereq.url ~ "^/count") {
            if (bereq.retries >= 3) {
                set bereq.backend = httpbuf;
                return(retry);
            }

            vtc.sleep(300ms * (bereq.retries + 1));
            return(retry);
        }
    }

It's intended as a emergency effort to prevent downtime for some critical
requests.

A `http.Request` takes up about 13k, so buffering 100k requests takes about 128M
of memory. This of course increases with large requests sizes etc.

Install with `go get zgo.at/httpbuf`. Configuration is done by editing config.go
and recompiling.
