package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
	apiextensionsv1types "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

type ResourceInfo struct {
	GVR       schema.GroupVersionResource
	Name      string
	Namespace string
	Kind      string
}

type KubeClient struct {
	clientset           *kubernetes.Clientset
	apiextensionsClient *apiextensionsv1.Clientset
	dynamicClient       dynamic.Interface
	discovery           discovery.DiscoveryInterface
	rateLimiter         *rate.Limiter
	adaptiveDelay       time.Duration
	delayMutex          sync.Mutex
}

var (
	verbose = flag.Bool("verbose", false, "Enable verbose logging including rate limit warnings")
)

func main() {
	flag.Parse()

	// Configure klog based on verbose flag.
	klog.InitFlags(nil)
	if *verbose {
		flag.Set("v", "2")
		log.Println("Verbose logging enabled")
	} else {
		// Suppress klog output (rate limit warnings).
		flag.Set("logtostderr", "false")
		flag.Set("alsologtostderr", "false")
		flag.Set("stderrthreshold", "3") // Only show FATAL.
	}

	log.Println("Starting bpfman purge from OpenShift cluster...")

	// Create context with signal handling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT/SIGTERM.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Received termination signal, cancelling operations...")
		cancel()
	}()

	// Add timeout.
	ctx, cancel = context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// Initialize Kubernetes client.
	client, err := initKubeClient()
	if err != nil {
		log.Fatalf("Failed to initialize Kubernetes client: %v", err)
	}

	if err := purgeBpfmanTargeted(ctx, client); err != nil {
		log.Fatalf("Failed to purge bpfman: %v", err)
	}

	log.Println("bpfman purge completed successfully!")
}

func initKubeClient() (*KubeClient, error) {
	// Use in-cluster config if available, otherwise use kubeconfig
	config, err := rest.InClusterConfig()
	if err != nil {
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			kubeconfigPath = clientcmd.RecommendedHomeFile
		}

		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	apiextensionsClient, err := apiextensionsv1.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create apiextensions client: %w", err)
	}

	discovery := clientset.Discovery()
	rateLimiter := rate.NewLimiter(rate.Limit(20), 50)

	return &KubeClient{
		clientset:           clientset,
		apiextensionsClient: apiextensionsClient,
		dynamicClient:       dynamicClient,
		discovery:           discovery,
		rateLimiter:         rateLimiter,
		adaptiveDelay:       100 * time.Millisecond,
		delayMutex:          sync.Mutex{},
	}, nil
}

func (c *KubeClient) adaptiveWait(ctx context.Context) error {
	c.delayMutex.Lock()
	delay := c.adaptiveDelay
	c.delayMutex.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func (c *KubeClient) adjustDelay(wasThrottled bool, retryAfter time.Duration) {
	c.delayMutex.Lock()
	defer c.delayMutex.Unlock()

	if wasThrottled {
		if retryAfter > 0 {
			c.adaptiveDelay = retryAfter
			if *verbose {
				log.Printf("Adaptive delay set to %v based on server retry-after", retryAfter)
			}
		} else {
			c.adaptiveDelay = min(time.Duration(float64(c.adaptiveDelay)*1.5), 5*time.Second)
			if *verbose {
				log.Printf("Adaptive delay increased to %v due to throttling", c.adaptiveDelay)
			}
		}
	} else {
		c.adaptiveDelay = max(time.Duration(float64(c.adaptiveDelay)*0.9), 50*time.Millisecond)
	}
}

func (c *KubeClient) executeWithAdaptiveRetry(ctx context.Context, operation func() error) error {
	maxRetries := 3
	for attempt := range maxRetries {
		if err := c.adaptiveWait(ctx); err != nil {
			return err
		}

		err := operation()
		if err == nil {
			c.adjustDelay(false, 0)
			return nil
		}

		if isRateLimitError(err) {
			retryAfter := extractRetryAfter(err)
			c.adjustDelay(true, retryAfter)

			if attempt < maxRetries-1 {
				if *verbose {
					log.Printf("Rate limited, retrying in %v (attempt %d/%d)", c.adaptiveDelay, attempt+1, maxRetries)
				}
				continue
			}
		}

		return err
	}

	return fmt.Errorf("max retries exceeded")
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "throttl") ||
		strings.Contains(errStr, "too many requests") ||
		strings.Contains(errStr, "429")
}

func extractRetryAfter(err error) time.Duration {
	if err == nil {
		return 0
	}

	errStr := err.Error()

	// Look for "retry-after: 1s" pattern
	if strings.Contains(errStr, "retry-after:") {
		parts := strings.Split(errStr, "retry-after:")
		if len(parts) > 1 {
			retryPart := strings.TrimSpace(parts[1])
			if strings.HasSuffix(retryPart, "s") {
				retryPart = strings.TrimSuffix(retryPart, "s")
				retryPart = strings.Fields(retryPart)[0]
				if duration, err := time.ParseDuration(retryPart + "s"); err == nil {
					return duration
				}
			}
		}
	}

	return 0
}

func purgeBpfmanTargeted(ctx context.Context, client *KubeClient) error {
	log.Println("Step 1: Discovering bpfman resources...")

	// Discover all bpfman resources using targeted approach
	allResources, err := discoverBpfmanResourcesTargeted(ctx, client)
	if err != nil {
		return fmt.Errorf("failed to discover bpfman resources: %w", err)
	}

	if len(allResources) == 0 {
		log.Println("No bpfman resources found - cluster appears clean")
		return nil
	}

	log.Printf("Found %d bpfman resources total", len(allResources))

	// Categorize resources by type for proper deletion order
	crds, instances, otherResources := categorizeResources(allResources)

	log.Println("Step 2: Deleting bpfman custom resource instances...")
	if len(instances) > 0 {
		log.Printf("Found %d custom resource instances to delete", len(instances))
		if err := deleteResourcesParallel(ctx, client, instances); err != nil {
			log.Printf("Warning: Failed to delete some instances: %v", err)
		}
	}

	log.Println("Step 3: Deleting other bpfman resources...")
	if len(otherResources) > 0 {
		log.Printf("Found %d other resources to delete", len(otherResources))
		if err := deleteResourcesParallel(ctx, client, otherResources); err != nil {
			log.Printf("Warning: Failed to delete some resources: %v", err)
		}
	}

	log.Println("Step 4: Deleting bpfman CRDs...")
	if len(crds) > 0 {
		log.Printf("Found %d CRDs to delete", len(crds))
		if err := deleteResourcesParallel(ctx, client, crds); err != nil {
			log.Printf("Warning: Failed to delete some CRDs: %v", err)
		}
	}

	log.Println("Step 5: Final verification...")
	return verifyCleanup(ctx, client)
}

func discoverBpfmanResourcesTargeted(ctx context.Context, client *KubeClient) ([]ResourceInfo, error) {
	var allResources []ResourceInfo
	g, ctx := errgroup.WithContext(ctx)

	// Step 1: Find bpfman CRDs first
	log.Println("  Discovering bpfman CRDs...")
	crds, err := getBpfmanCRDs(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to get bpfman CRDs: %w", err)
	}
	allResources = append(allResources, crds...)
	log.Printf("  Found %d bpfman CRDs", len(crds))

	// Step 2: Find instances of bpfman CRDs in parallel
	log.Println("  Discovering bpfman CRD instances...")
	var instanceResults [][]ResourceInfo
	instanceResults = make([][]ResourceInfo, len(crds))

	for i, crd := range crds {
		i, crd := i, crd
		g.Go(func() error {
			instances, err := getCRDInstances(ctx, client, crd)
			if err != nil {
				log.Printf("  Warning: Failed to get instances of %s: %v", crd.Name, err)
				return nil
			}
			instanceResults[i] = instances
			log.Printf("  Found %d instances of %s", len(instances), crd.Name)
			return nil
		})
	}

	// Step 3 & 4: Run label and name discovery in parallel
	var labelResources []ResourceInfo
	var nameResources []ResourceInfo

	g.Go(func() error {
		log.Println("  Discovering bpfman resources by labels...")
		resources, err := getBpfmanResourcesByLabels(ctx, client)
		if err != nil {
			log.Printf("  Warning: Failed to get resources by labels: %v", err)
			return nil
		}
		labelResources = resources
		log.Printf("  Found %d resources by labels", len(resources))
		return nil
	})

	g.Go(func() error {
		log.Println("  Discovering bpfman resources by name patterns...")
		resources, err := getBpfmanResourcesByNames(ctx, client)
		if err != nil {
			log.Printf("  Warning: Failed to get resources by names: %v", err)
			return nil
		}
		nameResources = resources
		log.Printf("  Found %d resources by names", len(resources))
		return nil
	})

	// Wait for all parallel operations to complete
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Collect CRD instances
	for _, instances := range instanceResults {
		allResources = append(allResources, instances...)
	}

	// Collect label and name resources
	allResources = append(allResources, labelResources...)
	allResources = append(allResources, nameResources...)

	// Deduplicate resources
	allResources = deduplicateResources(allResources)

	// Log summary
	if len(allResources) > 0 {
		log.Println("  Summary of discovered resources:")
		resourceCounts := make(map[string]int)
		for _, resource := range allResources {
			key := resource.GVR.Resource
			resourceCounts[key]++
		}
		for resourceType, count := range resourceCounts {
			log.Printf("    %s: %d", resourceType, count)
		}
	}

	return allResources, nil
}

func getBpfmanCRDs(ctx context.Context, client *KubeClient) ([]ResourceInfo, error) {
	var crds []ResourceInfo

	bpfmanCRDNames := []string{
		"bpfapplications.bpfman.io",
		"bpfapplicationstates.bpfman.io",
		"clusterbpfapplications.bpfman.io",
		"clusterbpfapplicationstates.bpfman.io",
	}

	bpfmanLabelSelectors := []string{
		"app.kubernetes.io/name=bpfman",
		"app.kubernetes.io/part-of=bpfman",
		"app=bpfman",
	}

	// First, find CRDs by label selectors
	log.Printf("    Checking CRDs with label selectors...")
	for _, selector := range bpfmanLabelSelectors {
		err := client.executeWithAdaptiveRetry(ctx, func() error {
			crdList, err := client.apiextensionsClient.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				return err
			}

			for _, crd := range crdList.Items {
				log.Printf("    Found bpfman CRD: %s (group: %s) - by label selector: %s", crd.Name, crd.Spec.Group, selector)
				crds = append(crds, ResourceInfo{
					GVR: schema.GroupVersionResource{
						Group:    "apiextensions.k8s.io",
						Version:  "v1",
						Resource: "customresourcedefinitions",
					},
					Name:      crd.Name,
					Namespace: "",
					Kind:      "CustomResourceDefinition",
				})
			}
			return nil
		})
		if err != nil {
			log.Printf("    Warning: Failed to list CRDs with selector %s: %v", selector, err)
			continue
		}
	}

	// Then, get all CRDs and filter by name patterns (fallback)
	log.Printf("    Checking CRDs by name patterns...")
	var crdList *apiextensionsv1types.CustomResourceDefinitionList
	err := client.executeWithAdaptiveRetry(ctx, func() error {
		var err error
		crdList, err = client.apiextensionsClient.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
		return err
	})
	if err != nil {
		return nil, err
	}

	for _, crd := range crdList.Items {
		crdName := strings.ToLower(crd.Name)
		crdGroup := strings.ToLower(crd.Spec.Group)

		// Check if this CRD is bpfman-related
		isBpfmanCRD := slices.Contains(bpfmanCRDNames, crdName)

		// Also check for name/group patterns
		if !isBpfmanCRD {
			isBpfmanCRD = strings.Contains(crdName, "bpfman") ||
				strings.Contains(crdGroup, "bpfman") ||
				strings.Contains(crdName, "bpfapplication") ||
				strings.Contains(crdName, "xdpprogram") ||
				strings.Contains(crdName, "tcprogram") ||
				strings.Contains(crdName, "tracepointprogram") ||
				strings.Contains(crdName, "kprobeprogram") ||
				strings.Contains(crdName, "uprobeprogram") ||
				strings.Contains(crdName, "fentryprogram") ||
				strings.Contains(crdName, "fexitprogram")
		}

		if isBpfmanCRD {
			log.Printf("    Found bpfman CRD: %s (group: %s) - by name pattern", crd.Name, crd.Spec.Group)
			crds = append(crds, ResourceInfo{
				GVR: schema.GroupVersionResource{
					Group:    "apiextensions.k8s.io",
					Version:  "v1",
					Resource: "customresourcedefinitions",
				},
				Name:      crd.Name,
				Namespace: "",
				Kind:      "CustomResourceDefinition",
			})
		}
	}

	// Deduplicate CRDs
	return deduplicateResources(crds), nil
}

func getCRDInstances(ctx context.Context, client *KubeClient, crd ResourceInfo) ([]ResourceInfo, error) {
	var instances []ResourceInfo

	// Get the CRD to understand its structure
	var crdObj *apiextensionsv1types.CustomResourceDefinition
	err := client.executeWithAdaptiveRetry(ctx, func() error {
		var err error
		crdObj, err = client.apiextensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
		return err
	})
	if err != nil {
		return nil, err
	}

	// Create GVR for the custom resource
	gvr := schema.GroupVersionResource{
		Group:    crdObj.Spec.Group,
		Version:  crdObj.Spec.Versions[0].Name,
		Resource: crdObj.Spec.Names.Plural,
	}

	// List all instances across all namespaces
	var list *unstructured.UnstructuredList
	err = client.executeWithAdaptiveRetry(ctx, func() error {
		var err error
		list, err = client.dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
		return err
	})
	if err != nil {
		return nil, err
	}

	for _, instance := range list.Items {
		instances = append(instances, ResourceInfo{
			GVR:       gvr,
			Name:      instance.GetName(),
			Namespace: instance.GetNamespace(),
			Kind:      crdObj.Spec.Names.Kind,
		})
	}

	return instances, nil
}

func getBpfmanResourcesByLabels(ctx context.Context, client *KubeClient) ([]ResourceInfo, error) {
	var allResources []ResourceInfo

	// Define specific resource types to check with label selectors
	targetResourceTypes := []schema.GroupVersionResource{
		// Core resources
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "", Version: "v1", Resource: "serviceaccounts"},
		{Group: "", Version: "v1", Resource: "configmaps"},
		{Group: "", Version: "v1", Resource: "secrets"},

		// Apps resources
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Group: "apps", Version: "v1", Resource: "replicasets"},

		// RBAC resources
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},

		// OLM resources
		{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "subscriptions"},
		{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "clusterserviceversions"},
		{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "catalogsources"},
		{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "operatorgroups"},

		// Monitoring resources
		{Group: "monitoring.coreos.com", Version: "v1", Resource: "servicemonitors"},
	}

	bpfmanSelectors := []string{
		"app.kubernetes.io/name=bpfman",
		"app.kubernetes.io/part-of=bpfman",
		"app=bpfman",
	}

	g, ctx := errgroup.WithContext(ctx)
	var resourcesMutex sync.Mutex

	// Create a semaphore to limit concurrent operations
	semaphore := make(chan struct{}, 10) // Allow up to 10 concurrent operations

	for _, gvr := range targetResourceTypes {
		for _, selector := range bpfmanSelectors {
			gvr, selector := gvr, selector
			g.Go(func() error {
				select {
				case semaphore <- struct{}{}:
					defer func() { <-semaphore }()
				case <-ctx.Done():
					return ctx.Err()
				}

				resources, err := findResourcesWithSelector(ctx, client, gvr, selector)
				if err != nil {
					return nil // Skip if this resource type doesn't exist or selector fails
				}

				if len(resources) > 0 {
					resourcesMutex.Lock()
					allResources = append(allResources, resources...)
					resourcesMutex.Unlock()
				}

				return nil
			})
		}
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return allResources, nil
}

func getBpfmanResourcesByNames(ctx context.Context, client *KubeClient) ([]ResourceInfo, error) {
	var allResources []ResourceInfo

	// Define specific resource types to check by name
	targetResourceTypes := []schema.GroupVersionResource{
		// Core resources
		{Group: "", Version: "v1", Resource: "services"},
		{Group: "", Version: "v1", Resource: "serviceaccounts"},
		{Group: "", Version: "v1", Resource: "configmaps"},
		{Group: "", Version: "v1", Resource: "secrets"},
		{Group: "", Version: "v1", Resource: "namespaces"},

		// Apps resources
		{Group: "apps", Version: "v1", Resource: "deployments"},
		{Group: "apps", Version: "v1", Resource: "daemonsets"},
		{Group: "apps", Version: "v1", Resource: "replicasets"},

		// RBAC resources
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"},

		// OLM resources
		{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "subscriptions"},
		{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "clusterserviceversions"},
		{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "catalogsources"},
		{Group: "operators.coreos.com", Version: "v1alpha1", Resource: "operatorgroups"},

		// Monitoring resources
		{Group: "monitoring.coreos.com", Version: "v1", Resource: "servicemonitors"},
	}

	g, ctx := errgroup.WithContext(ctx)
	var resourcesMutex sync.Mutex

	// Create a semaphore to limit concurrent operations
	semaphore := make(chan struct{}, 10) // Allow up to 10 concurrent operations

	for _, gvr := range targetResourceTypes {
		gvr := gvr
		g.Go(func() error {
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return ctx.Err()
			}

			resources, err := findResourcesByNamePattern(ctx, client, gvr)
			if err != nil {
				return nil // Skip if this resource type doesn't exist
			}

			if len(resources) > 0 {
				resourcesMutex.Lock()
				allResources = append(allResources, resources...)
				resourcesMutex.Unlock()
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return allResources, nil
}

func findResourcesWithSelector(ctx context.Context, client *KubeClient, gvr schema.GroupVersionResource, selector string) ([]ResourceInfo, error) {
	var resources []ResourceInfo

	// Try cluster-scoped first
	var list *unstructured.UnstructuredList
	err := client.executeWithAdaptiveRetry(ctx, func() error {
		var err error
		list, err = client.dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		return err
	})
	if err != nil {
		return nil, err
	}

	for _, item := range list.Items {
		resourceName := item.GetName()
		if isBpfmanResource(resourceName) {
			resources = append(resources, ResourceInfo{
				GVR:       gvr,
				Name:      resourceName,
				Namespace: item.GetNamespace(),
				Kind:      item.GetKind(),
			})
		}
	}

	return resources, nil
}

func findResourcesByNamePattern(ctx context.Context, client *KubeClient, gvr schema.GroupVersionResource) ([]ResourceInfo, error) {
	var resources []ResourceInfo

	// Try cluster-scoped first
	var list *unstructured.UnstructuredList
	err := client.executeWithAdaptiveRetry(ctx, func() error {
		var err error
		list, err = client.dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
		return err
	})
	if err != nil {
		return nil, err
	}

	for _, item := range list.Items {
		resourceName := item.GetName()
		if isBpfmanResource(resourceName) {
			resources = append(resources, ResourceInfo{
				GVR:       gvr,
				Name:      resourceName,
				Namespace: item.GetNamespace(),
				Kind:      item.GetKind(),
			})
		}
	}

	return resources, nil
}

func isBpfmanResource(name string) bool {
	nameLower := strings.ToLower(name)
	return strings.Contains(nameLower, "bpfman") ||
		strings.Contains(nameLower, "bpfapplication") ||
		strings.Contains(nameLower, "xdpprogram") ||
		strings.Contains(nameLower, "tcprogram") ||
		strings.Contains(nameLower, "tracepointprogram") ||
		strings.Contains(nameLower, "kprobeprogram") ||
		strings.Contains(nameLower, "uprobeprogram") ||
		strings.Contains(nameLower, "fentryprogram") ||
		strings.Contains(nameLower, "fexitprogram")
}

func categorizeResources(resources []ResourceInfo) (crds, instances, others []ResourceInfo) {
	for _, resource := range resources {
		if resource.GVR.Resource == "customresourcedefinitions" {
			crds = append(crds, resource)
		} else if isCRDInstance(resource) {
			instances = append(instances, resource)
		} else {
			others = append(others, resource)
		}
	}
	return
}

func isCRDInstance(resource ResourceInfo) bool {
	bpfmanCRDResources := []string{
		"bpfapplications",
		"bpfapplicationstates",
		"clusterbpfapplications",
		"clusterbpfapplicationstates",
	}

	return slices.Contains(bpfmanCRDResources, resource.GVR.Resource)
}

func deleteResourcesParallel(ctx context.Context, client *KubeClient, resources []ResourceInfo) error {
	g, ctx := errgroup.WithContext(ctx)

	for _, resource := range resources {
		g.Go(func() error {
			return deleteResourceWithFinalizers(ctx, client, resource)
		})
	}

	return g.Wait()
}

func deleteResourceWithFinalizers(ctx context.Context, client *KubeClient, resource ResourceInfo) error {
	var location string
	if resource.Namespace != "" {
		location = fmt.Sprintf("%s/%s in namespace %s", resource.Kind, resource.Name, resource.Namespace)
	} else {
		location = fmt.Sprintf("%s/%s (cluster-scoped)", resource.Kind, resource.Name)
	}

	log.Printf("Deleting %s...", location)

	// Try normal deletion first
	if err := deleteResource(ctx, client, resource); err == nil {
		log.Printf("Successfully deleted %s", location)
		return nil
	}

	// Check and handle finalizers
	obj, err := getResource(ctx, client, resource)
	if err != nil {
		log.Printf("Could not get resource %s (may not exist): %v", location, err)
		return nil
	}

	finalizers := obj.GetFinalizers()
	if len(finalizers) > 0 {
		log.Printf("Found finalizers on %s: %v", location, finalizers)

		// Try to remove finalizers
		if err := removeFinalizers(ctx, client, resource); err != nil {
			return fmt.Errorf("failed to remove finalizers from %s: %w", location, err)
		}
		log.Printf("Successfully removed finalizers from %s", location)

		// Try deletion again
		if err := deleteResource(ctx, client, resource); err != nil {
			return fmt.Errorf("failed to delete %s after removing finalizers: %w", location, err)
		}
		log.Printf("Successfully deleted %s after removing finalizers", location)
	} else {
		// No finalizers, try once more
		if err := deleteResource(ctx, client, resource); err != nil {
			return fmt.Errorf("failed to delete %s: %w", location, err)
		}
		log.Printf("Successfully deleted %s on retry", location)
	}

	return nil
}

func getResource(ctx context.Context, client *KubeClient, resource ResourceInfo) (*unstructured.Unstructured, error) {
	if resource.Namespace != "" {
		return client.dynamicClient.Resource(resource.GVR).Namespace(resource.Namespace).Get(ctx, resource.Name, metav1.GetOptions{})
	} else {
		return client.dynamicClient.Resource(resource.GVR).Get(ctx, resource.Name, metav1.GetOptions{})
	}
}

func deleteResource(ctx context.Context, client *KubeClient, resource ResourceInfo) error {
	deleteOptions := metav1.DeleteOptions{
		GracePeriodSeconds: &[]int64{0}[0],
	}

	if resource.Namespace != "" {
		return client.dynamicClient.Resource(resource.GVR).Namespace(resource.Namespace).Delete(ctx, resource.Name, deleteOptions)
	} else {
		return client.dynamicClient.Resource(resource.GVR).Delete(ctx, resource.Name, deleteOptions)
	}
}

func removeFinalizers(ctx context.Context, client *KubeClient, resource ResourceInfo) error {
	if resource.Namespace != "" {
		obj, err := client.dynamicClient.Resource(resource.GVR).Namespace(resource.Namespace).Get(ctx, resource.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		obj.SetFinalizers([]string{})

		_, err = client.dynamicClient.Resource(resource.GVR).Namespace(resource.Namespace).Update(ctx, obj, metav1.UpdateOptions{})
		return err
	} else {
		obj, err := client.dynamicClient.Resource(resource.GVR).Get(ctx, resource.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		obj.SetFinalizers([]string{})

		_, err = client.dynamicClient.Resource(resource.GVR).Update(ctx, obj, metav1.UpdateOptions{})
		return err
	}
}

func deduplicateResources(resources []ResourceInfo) []ResourceInfo {
	seen := make(map[string]bool)
	var result []ResourceInfo

	for _, resource := range resources {
		key := fmt.Sprintf("%s/%s/%s/%s", resource.GVR.String(), resource.Namespace, resource.Name, resource.Kind)
		if !seen[key] {
			seen[key] = true
			result = append(result, resource)
		}
	}

	return result
}

func verifyCleanup(ctx context.Context, client *KubeClient) error {
	// Re-run discovery to check if anything remains
	remaining, err := discoverBpfmanResourcesTargeted(ctx, client)
	if err != nil {
		log.Printf("Warning: Failed to verify cleanup: %v", err)
		return nil
	}

	if len(remaining) == 0 {
		log.Println("Cleanup verification: No bpfman resources remaining - SUCCESS")
	} else {
		log.Printf("Cleanup verification: %d bpfman resources still remain:", len(remaining))
		for _, resource := range remaining {
			if resource.Namespace != "" {
				log.Printf("  - %s/%s in namespace %s", resource.Kind, resource.Name, resource.Namespace)
			} else {
				log.Printf("  - %s/%s (cluster-scoped)", resource.Kind, resource.Name)
			}
		}
	}

	return nil
}
