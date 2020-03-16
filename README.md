# Spot Resource Mutator

[WIP]

Spot resource mutator is a server which intercepts API requests and validates resource requirements definition within the Pod Spec. It is done by configuring the [dynamic admission controller](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) to send webhook mutating requests to this server before the request is being persisted in K8s. It integrates with your Spot account and validates the resource requirements defined by the user. If there is no definition, the resource will be mutated with the appropriate values from the Spot backend relevant to the user Ocean cluster.

## Deploy on your cluster

Please verify that the target cluster is integrated with [Ocean](https://spotinst.com/products/ocean/), as this mutator will need the Spot account ID from the Ocean controller configuration (which should be existed in the cluster).

The deployment process will generate a Self Signed Certificate and will create a secret in your cluster for the resource mutator server

```bash
make gencerts-deploy
```

## Run locally

To run the command locally, you have to create the certs for the server:

```bash
make gencerts-deploy
```

In addition to the certs above, the following ENV VAR should be set:

- SPOTINST_CONTROLLER_ID - the controller id for the Ocean cluster
- SPOTINST_TOKEN - token for Spot API
- SPOTINST_ACCOUNT - Spot account which the ocean cluster exists in

```bash
make gencerts-deploy
make runlocal
```
