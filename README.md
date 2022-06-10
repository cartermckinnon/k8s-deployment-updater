# Kubernetes deployment updator

This is a silly little program that will update an `image` in a Kubernetes deployment to the latest digest available from an upstream image.

For example, you may want a deployment to be automatically updated to use the current digest referred to by a `latest` tag.

Use this with caution. You likely only want this for a truly stateless application that does not serve critical production traffic.

#### On the command line:
```
usage: k8s-deployment-updator DEPLOYMENT_NAME IMAGE_REF

arguments:
  DEPLOYMENT_NAME
        Name of the deployment to be updated
  IMAGE_REF
        Reference of the upstream image

options:
  -in-cluster
        Use in-cluster Kubernetes authentication chain
  -kubeconfig string
        (optional) absolute path to the kubeconfig file (default "$HOME/.kube/config")
  -namespace string
        Namespace containing the targeted deployment (default "default")
```

#### In your Kubernetes cluster:
0. Take a look at [example.yaml](example.yaml).
1. You probably want to schedule regular (but not obsessive) checks for updates via a `CronJob`.
2. Make sure you enable in-cluster authentication with your Kubernetes control plane, and that you have a service account, role, and role binding to allow mutation of deployments.
3. You may need to mount an authentication token for your container registry, via a `dockerconfigjson` secret.