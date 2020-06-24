package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/mattbaird/jsonpatch"
	"github.com/spotinst/spotinst-sdk-go/service/ocean"
	"github.com/spotinst/spotinst-sdk-go/service/ocean/providers/aws"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/klog"
)

type containerResourceRatio struct {
	CPURatio float64
	MEMRatio float64
}

const (
	defaultCPU int = 100
	defaultMEM int = 256

	defaultAllowedCPURatio float64 = 0.2
	defaultAllowedMEMRatio float64 = 0.2

	deploymentPatchExample string = `[
{
    "op": "add",
    "path": "/spec/template/spec/containers/0/resources",
    "value": [{
        "limits": {
            "cpu": "345m",
            "memory": "567Mi"
        },
        "requests": {
            "cpu": "123m",
            "memory": "234Mi"
        }
    }]
},
{
    "op": "add",
    "path": "/spec/template/spec/containers/1/resources",
    "value": [{
        "limits": {
            "cpu": "555m",
            "memory": "567Mi"
        },
        "requests": {
            "cpu": "123m",
            "memory": "234Mi"
        }
    }]
}
]`
)

// mutate resource spec for deploymentsonly allow pods to pull images from specific registry.
func mutateDeploymentResources(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	klog.V(2).Info("admitting pods")
	var allowResponse bool = true

	deployResource := metav1.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	reviewResponse := v1beta1.AdmissionResponse{}

	if ar.Request.Resource != deployResource {
		err := fmt.Errorf("Expect resource to be %s \n Mutating request passed", deployResource)
		klog.Error(err)
		return toAdmissionResponse(true, err)
	}

	raw := ar.Request.Object.Raw
	origDeploy := &appsv1.Deployment{}
	modifiedDeploy := &appsv1.Deployment{}

	deserializer := codecs.UniversalDeserializer()
	if _, _, err := deserializer.Decode(raw, nil, origDeploy); err != nil {
		klog.Error(err)
		return toAdmissionResponse(true, err)
	}

	origDeploy.DeepCopyInto(modifiedDeploy)

	// if val, ok := modifiedDeploy.Annotations["spotinst.io/mutate-resource"]; ok && val == "enabled" {
	// klog.V(5).Info("spotinst.io/mutate-resource is enabled")

	spotSuggestedRequests, err := resourceSuggestions(modifiedDeploy.Name, modifiedDeploy.Namespace)
	if err != nil {
		klog.Error(err)
		return toAdmissionResponse(allowResponse, err)
	}

	// sum all mem and cpu
	var totalCPUMili int64
	var totalMEMMb int64
	for _, container := range modifiedDeploy.Spec.Template.Spec.Containers {
		totalCPUMili += container.Resources.Requests.Cpu().MilliValue()
		totalMEMMb += container.Resources.Requests.Memory().Value()
	}
	klog.V(4).Infof("total milicore for pod %d ", totalCPUMili)
	klog.V(4).Infof("total MB for pod - %d", totalMEMMb)

	// calc ratio for all containers
	contRatio := make(map[string]*containerResourceRatio)
	for _, container := range modifiedDeploy.Spec.Template.Spec.Containers {
		cpuRatio := float64(container.Resources.Requests.Cpu().MilliValue()) / float64(totalCPUMili)
		memRatio := float64(container.Resources.Requests.Memory().Value()) / float64(totalMEMMb)
		klog.V(4).Infof("cpuRatio = %d , memRatio = %d", cpuRatio, memRatio)
		contRatio[container.Name] = &containerResourceRatio{
			CPURatio: cpuRatio,
			MEMRatio: memRatio,
		}
	}

	spotRecommendedCPU := spotSuggestedRequests.Cpu().MilliValue()
	spotRecommendedMEM := spotSuggestedRequests.Memory().Value()

	for i, container := range modifiedDeploy.Spec.Template.Spec.Containers {
		{
			_ = i
			klog.V(4).Infof("Validating resources for container %s \n", container.Name)
			r := modifiedDeploy.Spec.Template.Spec.Containers[i].Resources

			reqCPU := container.Resources.Requests.Cpu().MilliValue()
			cpu := reqCPU

			reqMEM := container.Resources.Requests.Memory().Value()
			mem := reqMEM

			if container.Resources.Requests.Cpu().CmpInt64(0) == 0 {
				cpu = int64(math.Ceil(float64(spotRecommendedCPU) * contRatio[container.Name].CPURatio))
			} else {
				if ((1-defaultAllowedCPURatio)*float64(reqCPU) > float64(spotRecommendedCPU) ||
					(1+defaultAllowedCPURatio)*float64(reqCPU) < float64(spotRecommendedCPU)) &&
					spotRecommendedCPU != 0 {
					cpu = int64(math.Ceil(float64(spotRecommendedCPU) * contRatio[container.Name].CPURatio))
				}
			}
			klog.V(4).Infof("Calculated cpu = %d", cpu)

			if container.Resources.Requests.Memory().CmpInt64(0) == 0 {
				mem = int64(math.Ceil(float64(spotRecommendedMEM) * contRatio[container.Name].MEMRatio))
			} else {
				if ((1-defaultAllowedMEMRatio)*float64(reqMEM) > float64(spotRecommendedMEM) ||
					(1+defaultAllowedMEMRatio)*float64(reqMEM) < float64(spotRecommendedMEM)) &&
					spotRecommendedMEM != 0 {
					mem = int64(math.Ceil(float64(spotRecommendedMEM) * contRatio[container.Name].MEMRatio))
				}
			}
			// Convert byte to MiB (Mebibyte = 2^20)
			mem = int64(math.Ceil(float64(mem) / float64(1048576)))
			klog.V(4).Infof("Calculated mem = %d", mem)

			mutatedCPUResrouce := resource.NewMilliQuantity(cpu, resource.DecimalSI)

			if err != nil {
				klog.Errorf("Cannot parse cpu milicore quantity of %d", cpu)
				*mutatedCPUResrouce = container.Resources.Requests[corev1.ResourceCPU]
			}
			klog.V(5).Infof("mutatedCPUResrouce = %v", mutatedCPUResrouce)

			mutatedMEMResrouce := resource.MustParse(strconv.FormatInt(mem, 10) + "Mi")
			if err != nil {
				klog.Errorf("Cannot parse memory milicore quantity of %d", mem)
				mutatedMEMResrouce = container.Resources.Requests[corev1.ResourceMemory]
			}
			klog.V(5).Infof("mutatedMEMResrouce = %v", mutatedMEMResrouce)

			r.Requests = corev1.ResourceList{
				corev1.ResourceCPU:    *mutatedCPUResrouce,
				corev1.ResourceMemory: mutatedMEMResrouce,
			}

			modifiedDeploy.Spec.Template.Spec.Containers[i].Resources = r
		}

	}

	origDeployByte, err := json.Marshal(origDeploy)
	if err != nil {
		klog.Error(err)
		return toAdmissionResponse(allowResponse, err)
	}
	modifiedDeployByte, err := json.Marshal(modifiedDeploy)
	if err != nil {
		klog.Error(err)
		return toAdmissionResponse(allowResponse, err)
	}

	klog.V(4).Infof("origDeployByte= %s", origDeployByte)
	klog.V(4).Infof("modifiedDeploy= %s", modifiedDeployByte)

	patch, err := jsonpatch.CreatePatch(origDeployByte, modifiedDeployByte)
	if err != nil {
		klog.Error(err)
		return toAdmissionResponse(allowResponse, err)
	}
	klog.V(4).Infof("patch= %s", patch)
	pb, _ := json.Marshal(patch)

	reviewResponse.Patch = pb
	pt := v1beta1.PatchTypeJSONPatch
	reviewResponse.PatchType = &pt
	// }

	reviewResponse.Allowed = allowResponse
	return &reviewResponse
}

func resourceSuggestions(deployment, namespace string) (*corev1.ResourceList, error) {

	ctx := context.Background()
	svc := ocean.New(DefaultConfig.spotSession, DefaultConfig.spotSession.Config)

	clusterOut, err := svc.CloudProviderAWS().ListClusters(ctx, &aws.ListClustersInput{})
	if err != nil {
		klog.Error(err)
		return nil, errors.New("Failed to list clusters")
	}
	if clusterOut.Clusters != nil {
		for _, cluster := range clusterOut.Clusters {
			klog.V(4).Infof("Got ControllerClusterID=%s", *cluster.ControllerClusterID)
			if *cluster.ControllerClusterID == DefaultConfig.ControllerClusterID {
				return oceanResourceSuggestions(ctx, svc, cluster, deployment, namespace)
			}

		}
	}
	return nil, errors.New("No cluster was found with 'ControllerClusterID' = " + DefaultConfig.ControllerClusterID)
}

func oceanResourceSuggestions(context context.Context, svc *ocean.ServiceOp, ocean *aws.Cluster, deployment, namespace string) (*corev1.ResourceList, error) {

	rsSuggetsions, err := svc.CloudProviderAWS().ListResourceSuggestions(context, &aws.ListResourceSuggestionsInput{
		OceanID:   ocean.ID,
		Namespace: &namespace,
	})

	if err != nil {
		klog.Error(err)
		return nil, errors.New("Failed to get resource suggestions for ocean")
	}

	for _, suggestion := range rsSuggetsions.Suggestions {
		if *suggestion.DeploymentName == deployment {
			klog.V(4).Infof("Found deployment=%s", *suggestion.DeploymentName)

			if suggestion.SuggestedCPU != nil && suggestion.SuggestedMemory == nil {
				klog.V(4).Infof("suggested memory is nil")

				return &corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse(strconv.Itoa(*suggestion.SuggestedCPU) + "m"),
				}, nil
			}

			if suggestion.SuggestedCPU == nil && suggestion.SuggestedMemory != nil {
				klog.V(4).Infof("suggested cpu is nil")
				return &corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse(strconv.Itoa(*suggestion.SuggestedMemory) + "Mi"),
				}, nil
			}
			klog.V(4).Infof("suggested cpu is %d", *suggestion.SuggestedCPU)
			klog.V(4).Infof("suggested mem is %d", *suggestion.SuggestedMemory)

			return &corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(strconv.Itoa(*suggestion.SuggestedCPU) + "m"),
				corev1.ResourceMemory: resource.MustParse(strconv.Itoa(*suggestion.SuggestedMemory) + "Mi"),
			}, nil
		}
	}
	return nil, errors.New("No resource suggestions found for deployment - " + deployment)
}

func nextPO2(v int64) int64 {
	v--
	v |= v >> 1
	v |= v >> 2
	v |= v >> 4
	v |= v >> 8
	v |= v >> 16
	v |= v >> 32
	v++
	return v
}
