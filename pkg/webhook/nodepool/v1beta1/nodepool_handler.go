/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/webhook/util"
)

// SetupWebhookWithManager sets up Cluster webhooks. 	mutate path, validatepath, error
func (webhook *NodePoolHandler) SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error) {
	// init
	webhook.Client = mgr.GetClient()

	gvk, err := apiutil.GVKForObject(&v1beta1.NodePool{}, mgr.GetScheme())
	if err != nil {
		return "", "", err
	}
	return util.GenerateMutatePath(gvk),
		util.GenerateValidatePath(gvk),
		ctrl.NewWebhookManagedBy(mgr).
			For(&v1beta1.NodePool{}).
			WithDefaulter(webhook).
			WithValidator(webhook).
			Complete()
}

// +kubebuilder:webhook:verbs=create;update;delete,path=/validate-apps-openyurt-io-v1beta1-nodepool,mutating=false,failurePolicy=fail,groups=apps.openyurt.io,resources=nodepools,versions=v1beta1,name=v.v1beta1.nodepool.kb.io,sideEffects=None,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/mutate-apps-openyurt-io-v1beta1-nodepool,mutating=true,failurePolicy=fail,groups=apps.openyurt.io,resources=nodepools,verbs=create;update,versions=v1beta1,name=m.v1beta1.nodepool.kb.io,sideEffects=None,admissionReviewVersions=v1

// NodePoolHandler implements a validating and defaulting webhook for Cluster.
type NodePoolHandler struct {
	Client client.Client
}

var _ webhook.CustomDefaulter = &NodePoolHandler{}
var _ webhook.CustomValidator = &NodePoolHandler{}
