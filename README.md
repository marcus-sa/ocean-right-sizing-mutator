# Spot Resource Mutator

Spot resource mutator is a server which intercepts API requests and validates resource requirements definition within the Pod Spec. 

This is done by configuring the [dynamic admission controller](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) to send webhook mutating requests to this server before the request is being persisted in K8s. The server integrates with your Spot account and validates the resource requirements defined by the user. If there is no definition, the resource will be mutated with the appropriate values from the Spot backend recommended by Ocean.

## Deploy

**Requirements**

* The target cluster must be integrated with [Ocean](https://spotinst.com/products/ocean/).
* The mutator requires a Spot account ID from the previously installed Ocean controller.

The deployment process will generate a Self Signed Certificate and will create a secret in your cluster for the resource mutator server

**Run** 

```bash
make gencerts-deploy
```

## Run locally

To run the command locally, create the certs for the server:

```bash
make gencerts-deploy
```

In addition to the certs above, the following ENV VARs should be set:

- SPOTINST_CONTROLLER_ID - the controller id for the Ocean cluster
- SPOTINST_TOKEN - token for Spot API
- SPOTINST_ACCOUNT - Spot account which the ocean cluster exists in

```bash
make gencerts-deploy
make runlocal
```
