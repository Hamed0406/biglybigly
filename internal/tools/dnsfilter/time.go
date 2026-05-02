package dnsfilter

import "time"

// timeNow is a package-level indirection over time.Now so tests can inject
// deterministic timestamps.
var timeNow = time.Now
