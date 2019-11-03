# config-reflector

config-reflector is a Kubernetes operator for sharing ConfigMap and Secret objects across namespaces.

## Features

- Update event propagation to reflected objects
- Automatic garbage collection

## Usage

1. Create a new namespace called `config-reflector`

    ```
    kubectl create ns config-reflector
    ```

1. Deploy manifests in `./deploy` directory

    ```
    kubectl apply -f ./deploy
    ```

1. Check if operator pod is running in `config-reflector` namespace

    ```
    kubectl get pods -n config-reflector
    ```

1. Use following annotations to control how ConfigMaps and Secrets are reflected in other namespaces

    - `configreflector.github.io/reflect-namespaces: default, test`  
      This annotation is used to pass operator a comma separated list of namespaces where annotated Object should be reflected.
    - `configreflector.github.io/reflect-annotations: "true"`  
      This annotation tells operator whether it should copy annotations from source object to its reflection. By default, it is disabled. When enabled, operator related annotations are not copied.
    - `configreflector.github.io/reflect-labels: "true"`  
      This annotation tells operator whether it should copy labels from source object to its reflection. It is disabled by default.

## TODO

- [ ] Continuous Integration
- [ ] Unit Tests
- [ ] E2E Tests

## License

```
   Copyright 2019 Ashutosh Gangwar

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
```
