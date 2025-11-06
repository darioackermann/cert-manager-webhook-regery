# ACME Webhook for Regery API

This solver can be used when you want to use cert-manager with Regery. It follows the [documentation provided by Regery](https://regery.com/en/partnership/api/introduction).

Note that this Webhook has been written by a private person not associated with Regery.


## Installation

### cert-manager

Follow the [instructions](https://cert-manager.io/docs/installation/) using the cert-manager documentation to install it
within your cluster.

### Webhook

#### Using public helm chart

```bash
helm repo add cert-manager-webhook-regery https://darioackermann.github.io/cert-manager-webhook-regery
helm upgrade --install -n cert-manager cert-manager-webhook-regery cert-manager-webhook-regery/cert-manager-webhook-regery
```

#### From local checkout

```bash
helm install --namespace cert-manager cert-manager-webhook-regery deploy/cert-manager-webhook-regery
```

**Note**: The kubernetes resources used to install the webhook should be deployed within the same namespace as the
cert-manager.

To uninstall the webhook run

```bash
helm uninstall --namespace cert-manager cert-manager-webhook-regery
```

## Issuer

Create a new or extend your existing `ClusterIssuer`. An example is provided below.

```yaml
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-dns01-issuer
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: you@example.ch
    privateKeySecretRef:
      name: letsencrypt-dns01-private-key
    solvers:
    # ... existing solvers
    # Add the regery solver here
    - selector:
        dnsZones:
          - 'example.ch'
      dns01:
        webhook:
          groupName: acme.regery.com
          solverName: regery
          config:
            secretName: regery-secret
```

### API Keys

In order to access the Regery API, the webhook needs an API Key as well as an API Secret.  
Please create one and enable it in the [API Access](https://regery.com/control/account/api-access) section of the Regery Console.

If you choose another name for the secret than `regery-secret`, you must install the chart with a modified `secretName`
value. Policies ensure that no other secrets can be read by the webhook. Also modify the value of `secretName` in the
`ClusterIssuer`.

Create the secret using `kubectl`:

```shell
kubectl create secret generic regery-secret --namespace cert-manager \ 
  --from-literal=api-key=API_********** \ 
  --from-literal=api-secret='********'
```

### Create a certificate

Finally you can create certificates, for example:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-cert
  namespace: cert-manager
spec:
  commonName: example.ch
  dnsNames:
    - example.ch
  issuerRef:
    name: letsencrypt-dns01-issuer
    kind: ClusterIssuer
  secretName: example-cert
```

## Development

### Running the test suite

First, you need to have a Regery account with access to DNS control panel. You need to create API token and have a
registered DNS zone.
You also must encode your API Key and API Secret into base64 and put the hash into `testdata/regery/regery-secret.yml` file.

You can then run the test suite with:

```bash
TEST_ZONE_NAME=example.ch. make test
```

## Creating new package

To build new Docker image for multiple architectures and push it to hub:

```shell
docker buildx build --platform linux/amd64,linux/arm64,linux/arm/v7 -t darioackermann/cert-manager-webhook-regery:VERSION . --push
```

To compile and publish new Helm chart version:

```shell
helm package deploy/cert-manager-webhook-regery
git checkout gh-pages
helm repo index . --url https://darioackermann.github.io/cert-manager-webhook-regery/
```