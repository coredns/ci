module github.com/coredns/ci

go 1.12

require (
	github.com/coredns/caddy v1.1.0
	github.com/coredns/coredns v0.0.0
	github.com/miekg/dns v1.1.35
	github.com/prometheus/common v0.14.0
	k8s.io/api v0.19.2
	k8s.io/apimachinery v0.19.3
	k8s.io/client-go v0.19.2
)

replace github.com/coredns/coredns v0.0.0 => ../coredns
