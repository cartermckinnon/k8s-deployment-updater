package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/retry"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	registry "github.com/google/go-containerregistry/pkg/v1/remote"
)

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}

	inCluster := flag.Bool("in-cluster", false, "Use in-cluster Kubernetes authentication chain")

	namespace := flag.String("namespace", "default", "Namespace containing the targeted deployment")

	flag.Usage = func() {
		w := flag.CommandLine.Output()
		fmt.Fprintf(w, "usage: %s DEPLOYMENT_NAME IMAGE_REF\n", os.Args[0])
		fmt.Fprintf(w, "\narguments:\n")
		fmt.Fprintf(w, "  DEPLOYMENT_NAME\n        Name of the deployment to be updated\n")
		fmt.Fprintf(w, "  IMAGE_REF\n        Reference of the upstream image\n")
		fmt.Fprintf(w, "\noptions:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(1)
	}
	deploymentName := flag.Arg(0)
	imageReference, err := name.ParseReference(flag.Arg(1))
	if err != nil {
		panic(err)
	}

	image, err := registry.Image(imageReference, registry.WithAuthFromKeychain(authn.DefaultKeychain))
	if err != nil {
		panic(err)
	}
	digest, err := image.Digest()
	if err != nil {
		panic(err)
	}
	fmt.Println("latest image digest is", digest.Hex)

	var config *rest.Config
	if *inCluster {
		inClusterConfig, err := rest.InClusterConfig()
		if err != nil {
			panic(err)
		}
		config = inClusterConfig
	} else {
		flagConfig, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			panic(err)
		}
		config = flagConfig
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	deploymentRes := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Retrieve the latest version of Deployment before attempting update
		// RetryOnConflict uses exponential backoff to avoid exhausting the apiserver
		result, getErr := client.Resource(deploymentRes).Namespace(*namespace).Get(context.TODO(), deploymentName, metav1.GetOptions{})
		if getErr != nil {
			panic(fmt.Errorf("failed to get latest version of Deployment: %v", getErr))
		}

		// extract spec containers
		containers, found, err := unstructured.NestedSlice(result.Object, "spec", "template", "spec", "containers")
		if err != nil || !found || containers == nil {
			panic(fmt.Errorf("deployment containers not found or error in spec: %v", err))
		}

		for _, c := range containers {
			currentImageReference := c.(map[string]interface{})["image"].(string)
			parts := strings.Split(currentImageReference, "@")
			if len(parts) == 1 {
				parts = strings.Split(currentImageReference, ":")
			}
			currentImageName := parts[0]
			if currentImageName != imageReference.Context().Name() {
				continue
			}
			currentImageTag := "latest"
			if len(parts) == 2 {
				currentImageTag = parts[1]
			}

			latestDigestTag := "sha256:" + digest.Hex

			if currentImageTag == latestDigestTag {
				fmt.Println("deployment is up to date")
			} else {
				newImageReference := currentImageName + "@" + latestDigestTag
				if err := unstructured.SetNestedField(c.(map[string]interface{}), newImageReference, "image"); err != nil {
					panic(err)
				}
				if err := unstructured.SetNestedField(result.Object, containers, "spec", "template", "spec", "containers"); err != nil {
					panic(err)
				}
				_, updateErr := client.Resource(deploymentRes).Namespace(*namespace).Update(context.TODO(), result, metav1.UpdateOptions{})
				if updateErr != nil {
					panic(updateErr)
				}
				fmt.Printf("updated deployment from %s to %s\n", currentImageReference, newImageReference)
			}
			os.Exit(0)
		}

		panic(fmt.Errorf("failed to find container to update in deployment=%s", deploymentName))
	})
	if retryErr != nil {
		panic(fmt.Errorf("update failed: %v", retryErr))
	}
}
