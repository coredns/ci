package kubernetes

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/expfmt"
	api "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const namespace = "testns"

func TestDNSProgrammingLatencyEndpoints(t *testing.T) {
	corefile := `    .:53 {
        health
        ready
        errors
		prometheus :9153
        kubernetes cluster.local
    }
`
	err := LoadCorefile(corefile)
	if err != nil {
		t.Fatalf("Could not load corefile: %s", err)
	}

	config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		panic(err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	defer client.CoreV1().Namespaces().Delete(context.TODO(), namespace, meta.DeleteOptions{})
	if _, err := client.CoreV1().Namespaces().Create(context.TODO(), &api.Namespace{
		ObjectMeta: meta.ObjectMeta{Name: namespace},
	}, meta.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	// Create Services
	createService(t, client, "my-service", api.ClusterIPNone)
	createService(t, client, "clusterip-service", "10.96.99.12")
	createService(t, client, "headless-no-annotation", api.ClusterIPNone)
	createService(t, client, "headless-wrong-annotation", api.ClusterIPNone)

	// test endpoints and endpointslice
	t.Run("EndpointSlice", func(t *testing.T) { testEndpoints(t, client, true) })
	t.Run("Endpoint", func(t *testing.T) { testEndpoints(t, client, false) })
}

func testEndpoints(t *testing.T, client *kubernetes.Clientset, slices bool) {

	sv, _ := client.ServerVersion()
	major, _ := strconv.Atoi(sv.Major)
	minor, _ := strconv.Atoi(sv.Minor)

	if !slices && major >= 1 && minor >= 19 {
		// Skip tests for Endpoints if we are monitoring EndpointSlices (>= k8s 1.19).
		// Endpoint should be copied to EndpointSlices via the K8s EndpointSliceMirror
		// Controller. However, it may be unpredictably slow in the CI VMs and result
		// in flaky testing, or slow tests if we wait for it to complete.  We are already testing
		// EndpointSlices directly, so there is little value to testing Endpoints here.
		t.Skip("skipping for K8s versions >= 1.19")
		return
	}

	if slices && major <= 1 && minor < 19 {
		// Skip tests for EndpointSlices for k8s versions less that 1.19
		t.Skip("skipping for K8s versions < 1.19")
		return
	}

	// scrape and parse metrics to get base state
	m := ScrapeMetrics(t)
	var tp expfmt.TextParser
	base, err := tp.TextToMetricFamilies(strings.NewReader(string(m)))
	if err != nil {
		t.Fatalf("Could not parse scraped metrics: %v", err)
	}

	if slices {
		addUpdateEndpointSlice(t, client)
	} else {
		addUpdateEndpoints(t, client)
	}

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
		17: 1, // update for 1 endpoint/slice
		18: 2, // create for 1 endpoint/slice, plus previous bucket
		19: 2, // nothing new in bigger buckets
		20: 2,
	}
	if slices && major <= 1 && minor <= 18 {
		// CoreDNS does not watch endpointslices on k8s <= 1.18,
		// so expect to see no delta in histogram
		expectBucketsDelta = map[int]uint64{}
	}

	// create the expected bucket values by adding deltas to the base state buckets
	if _, ok := base[metricName]; ok {
		for i, bucket := range base[metricName].Metric[0].Histogram.Bucket {
			expectBuckets = append(expectBuckets, expectBucket{i, *bucket.CumulativeCount + expectBucketsDelta[i]})
		}
	}

	// scrape metrics and validate results
	m = ScrapeMetrics(t)
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

func addUpdateEndpoints(t *testing.T, client *kubernetes.Clientset) {
	subset1 := []api.EndpointSubset{{
		Addresses: []api.EndpointAddress{{IP: "1.2.3.6", Hostname: "foo"}},
		Ports:     []api.EndpointPort{{Port: 80, Name: "http"}},
	}}
	subset2 := []api.EndpointSubset{{
		Addresses: []api.EndpointAddress{{IP: "1.2.3.7", Hostname: "foo"}},
	}}
	createEndpoints(t, client, "my-service", time.Now().Add(-132*time.Second), subset1)
	updateEndpoints(t, client, "my-service", time.Now().Add(-66*time.Second), subset2)
	createEndpoints(t, client, "endpoints-no-service", time.Now().Add(-4*time.Second), nil)
	createEndpoints(t, client, "clusterip-service", time.Now().Add(-8*time.Second), nil)
	createEndpoints(t, client, "headless-no-annotation", nil, nil)
	createEndpoints(t, client, "headless-wrong-annotation", "wrong-value", nil)
}

func addUpdateEndpointSlice(t *testing.T, client *kubernetes.Clientset) {
	endpoints1 := []discovery.Endpoint{{
		Addresses: []string{"1.2.3.4"},
	}}
	endpoints2 := []discovery.Endpoint{{
		Addresses: []string{"1.2.3.5"},
	}}
	createEndpointSlice(t, client, "my-service", time.Now().Add(-132*time.Second), endpoints1)
	updateEndpointSlice(t, client, "my-service", time.Now().Add(-66*time.Second), endpoints2)
	createEndpointSlice(t, client, "endpoints-no-service", time.Now().Add(-4*time.Second), nil)
	createEndpointSlice(t, client, "clusterip-service", time.Now().Add(-8*time.Second), nil)
	createEndpointSlice(t, client, "headless-no-annotation", nil, nil)
	createEndpointSlice(t, client, "headless-wrong-annotation", "wrong-value", nil)
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
	_, err := client.DiscoveryV1().EndpointSlices(namespace).Create(ctx, buildEndpointSlice(name, triggerTime, endpoints), meta.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func updateEndpointSlice(t *testing.T, client kubernetes.Interface, name string, triggerTime interface{}, endpoints []discovery.Endpoint) {
	ctx := context.TODO()
	_, err := client.DiscoveryV1().EndpointSlices(namespace).Update(ctx, buildEndpointSlice(name, triggerTime, endpoints), meta.UpdateOptions{})
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
