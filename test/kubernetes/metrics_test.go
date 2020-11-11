package kubernetes

import (
	"context"
	"flag"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/expfmt"
	api "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1beta1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const namespace = "testns"

func TestDNSProgrammingLatencyEndpoints(t *testing.T) {

	var kubeconfig = "/home/circleci/.kube/kind-config-kind"

	flag.Parse()
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		panic(err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	endpoints1 := []discovery.Endpoint{{
		Addresses: []string{"1.2.3.4"},
	}}
	endpoints2 := []discovery.Endpoint{{
		Addresses: []string{"1.2.3.5"},
	}}
	subset1 := []api.EndpointSubset{{
		Addresses: []api.EndpointAddress{{IP: "1.2.3.6", Hostname: "foo"}},
		Ports:     []api.EndpointPort{{Port: 80, Name: "http"}},
	}}
	subset2 := []api.EndpointSubset{{
		Addresses: []api.EndpointAddress{{IP: "1.2.3.7", Hostname: "foo"}},
	}}

	defer client.CoreV1().Namespaces().Delete(context.TODO(), namespace, meta.DeleteOptions{})
	if _, err := client.CoreV1().Namespaces().Create(context.TODO(), &api.Namespace{
		ObjectMeta: meta.ObjectMeta{Name: namespace},
	}, meta.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	// scrape and parse metrics to get base state
	m := scrapeMetrics(t)
	var tp expfmt.TextParser
	base, err := tp.TextToMetricFamilies(strings.NewReader(string(m)))
	if err != nil {
		t.Fatalf("Could not parse scraped metrics: %v", err)
	}

	now := time.Now()

	createService(t, client, "my-service", api.ClusterIPNone)
	createEndpointSlice(t, client, "my-service", now.Add(-132*time.Second), endpoints1)
	updateEndpointSlice(t, client, "my-service", now.Add(-66*time.Second), endpoints2)
	createEndpoints(t, client, "my-service", now.Add(-132*time.Second), subset1)
	updateEndpoints(t, client, "my-service", now.Add(-66*time.Second), subset2)

	createEndpointSlice(t, client, "endpoints-no-service", now.Add(-4*time.Second), nil)
	createEndpoints(t, client, "endpoints-no-service", now.Add(-4*time.Second), nil)

	createService(t, client, "clusterip-service", "10.96.99.12")
	createEndpointSlice(t, client, "clusterip-service", now.Add(-8*time.Second), nil)
	createEndpoints(t, client, "clusterip-service", now.Add(-8*time.Second), nil)

	createService(t, client, "headless-no-annotation", api.ClusterIPNone)
	createEndpointSlice(t, client, "headless-no-annotation", nil, nil)
	createEndpoints(t, client, "headless-no-annotation", nil, nil)

	createService(t, client, "headless-wrong-annotation", api.ClusterIPNone)
	createEndpointSlice(t, client, "headless-wrong-annotation", "wrong-value", nil)
	createEndpoints(t, client, "headless-wrong-annotation", "wrong-value", nil)

	// give time for coredns to receive and process the events
	time.Sleep(time.Second)

	// prepare expected values
	metricName := "coredns_kubernetes_dns_programming_duration_seconds"
	type expectBucket struct {
		n     int
		count uint64
	}
	var expectBuckets []expectBucket

	// expectBucketsDelta holds the expected deltas in bucket counts after
	// the creates/updates in above tests
	expectBucketsDelta := map[int]uint64{
		17: 2, // update for 1 endpoint and 1 slice
		18: 4, // create for 1 endpoint and 1 slice, plus previous bucket
		19: 4, // nothing new in bigger buckets
		20: 4,
	}

	// create the expected bucket values by adding deltas to the base state buckets
	if _, ok := base[metricName]; ok {
		for i, bucket := range base[metricName].Metric[0].Histogram.Bucket {
			expectBuckets = append(expectBuckets, expectBucket{i, *bucket.CumulativeCount + expectBucketsDelta[i]})
		}
	}

	// scrape metrics and validate results
	m = scrapeMetrics(t)
	got, err := tp.TextToMetricFamilies(strings.NewReader(string(m)))
	if err != nil {
		t.Fatalf("Could not parse scraped metrics: %v", err)
	}

	if _, ok := got[metricName]; !ok {
		t.Fatalf("Did not find '%v' in scraped metrics.", metricName)
	}
	for _, eb := range expectBuckets {
		count := *got[metricName].Metric[0].Histogram.Bucket[eb.n].CumulativeCount
		if count != eb.count {
			t.Errorf("In bucket %v, expected %v, got %v", eb.n, eb.count, count)
		}
	}
}

func buildEndpoints(name string, lastChangeTriggerTime interface{}, subsets []api.EndpointSubset) *api.Endpoints {
	annotations := make(map[string]string)
	switch v := lastChangeTriggerTime.(type) {
	case string:
		annotations[api.EndpointsLastChangeTriggerTime] = v
	case time.Time:
		annotations[api.EndpointsLastChangeTriggerTime] = v.Format(time.RFC3339Nano)
	}
	return &api.Endpoints{
		ObjectMeta: meta.ObjectMeta{Namespace: namespace, Name: name, Annotations: annotations},
		Subsets:    subsets,
	}
}

func buildEndpointSlice(name string, lastChangeTriggerTime interface{}, endpoints []discovery.Endpoint) *discovery.EndpointSlice {
	annotations := make(map[string]string)
	switch v := lastChangeTriggerTime.(type) {
	case string:
		annotations[api.EndpointsLastChangeTriggerTime] = v
	case time.Time:
		annotations[api.EndpointsLastChangeTriggerTime] = v.Format(time.RFC3339Nano)
	}
	return &discovery.EndpointSlice{
		ObjectMeta: meta.ObjectMeta{
			Namespace: namespace, Name: name + "-12345",
			Labels:      map[string]string{discovery.LabelServiceName: name},
			Annotations: annotations,
		},
		AddressType: discovery.AddressTypeIPv4,
		Endpoints:   endpoints,
	}
}

func createEndpoints(t *testing.T, client kubernetes.Interface, name string, triggerTime interface{}, subsets []api.EndpointSubset) {
	ctx := context.TODO()
	_, err := client.CoreV1().Endpoints(namespace).Create(ctx, buildEndpoints(name, triggerTime, subsets), meta.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func updateEndpoints(t *testing.T, client kubernetes.Interface, name string, triggerTime interface{}, subsets []api.EndpointSubset) {
	ctx := context.TODO()
	_, err := client.CoreV1().Endpoints(namespace).Update(ctx, buildEndpoints(name, triggerTime, subsets), meta.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func createEndpointSlice(t *testing.T, client kubernetes.Interface, name string, triggerTime interface{}, endpoints []discovery.Endpoint) {
	ctx := context.TODO()
	_, err := client.DiscoveryV1beta1().EndpointSlices(namespace).Create(ctx, buildEndpointSlice(name, triggerTime, endpoints), meta.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func updateEndpointSlice(t *testing.T, client kubernetes.Interface, name string, triggerTime interface{}, endpoints []discovery.Endpoint) {
	ctx := context.TODO()
	_, err := client.DiscoveryV1beta1().EndpointSlices(namespace).Update(ctx, buildEndpointSlice(name, triggerTime, endpoints), meta.UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func createService(t *testing.T, client kubernetes.Interface, name string, clusterIp string) {
	ctx := context.TODO()
	if _, err := client.CoreV1().Services(namespace).Create(ctx, &api.Service{
		ObjectMeta: meta.ObjectMeta{Namespace: namespace, Name: name},
		Spec:       api.ServiceSpec{ClusterIP: clusterIp, Ports: []api.ServicePort{{Name: "http", Port: 80}}},
	}, meta.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func scrapeMetrics(t *testing.T) []byte {
	containerID, err := FetchDockerContainerID("kind-control-plane")
	if err != nil {
		t.Fatalf("docker container ID not found, err: %s", err)
	}

	ips, err := CoreDNSPodIPs()
	if err != nil {
		t.Errorf("could not get coredns pod ip: %v", err)
	}
	if len(ips) != 1 {
		t.Errorf("expected 1 pod ip, found: %v", len(ips))
	}

	ip := ips[0]
	cmd := fmt.Sprintf("docker exec -i %s /bin/sh -c \"curl -s http://%s:9153/metrics\"", containerID, ip)
	mf, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		t.Errorf("error while trying to run command in docker container: %s", err)
	}
	if len(mf) == 0 {
		t.Errorf("unable to scrape metrics from %v", ip)
	}
	return mf
}
