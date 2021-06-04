module github.com/coredns/ci

go 1.16

require (
	github.com/coredns/caddy v1.1.1
	github.com/coredns/coredns v0.0.0
	github.com/miekg/dns v1.1.42
	github.com/prometheus/common v0.25.0
	k8s.io/api v0.21.1
	k8s.io/apimachinery v0.21.1
	k8s.io/client-go v0.21.1
)

replace github.com/coredns/coredns v0.0.0 => ../coredns
