module github.com/coredns/ci

go 1.12

require (
	github.com/coredns/caddy v1.1.0
	github.com/coredns/coredns v0.0.0
	github.com/miekg/dns v1.1.30
)

replace github.com/coredns/coredns v0.0.0 => ../coredns
