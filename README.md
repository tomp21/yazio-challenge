# yazio-challenge
This repository holds the code for an operator responsible of managing a Redis single-master deployment, following the challenge proposed features:
- Manage Redis instances
- CRD Definition (Which can be found at TODO:link CRD here)
- Random password generation
- Secret management
- Redis deployment (Turned statefulsets due to the application's nature)
- Update and delete operations
- Documentation (This file will contain all required information or links to it)
- (Bonus) Basic tests
- (Bonus) Health watch mechanism/self healing capacity

## Getting Started

### Prerequisites
- go version v1.22.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster. ([kind](https://kind.sigs.k8s.io/) is recommended for local testing, as it was used for development)

### Deployment steps

Knowing our kubectl context is pointing to the cluster we want to deploy the operator to:

## Deployment using installer without cloning the repository
```
kubectl apply -f https://raw.githubusercontent.com/tomp21/yazio-challenge/main/dist/install.yaml
```

## Deployment manually creating operator components

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=ghcr.io/tomp21/yazio-challenge:main
```

And you will be ready to go!

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

### Creating a sample Redis instance 
You can apply the sample the config/sample:

```sh
kubectl apply -k config/samples/cache_v1alpha1_redis.yaml
```

### To Uninstall
**Delete the instances (CRs) from the cluster:**
```sh
kubectl delete -k config/samples/cache_v1alpha1_redis.yaml
```
If you renamed the resource after creation, you will need to delete it referncing its new name:

```sh
kubectl delete redis <whichever name you set to it>
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=ghcr.io/tomp21/yazio-challenge:main
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

